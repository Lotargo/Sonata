package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

type RuntimeRole string

const (
	RoleRouter           RuntimeRole = "router"
	RoleRaw              RuntimeRole = "raw"
	RoleCritical         RuntimeRole = "critical"
	RoleSummary          RuntimeRole = "summary"
	RoleSynthesisTooling RuntimeRole = "synthesis_tooling"
	RoleSynthesisFinal   RuntimeRole = "synthesis_final"
)

var ordinaryRuntimeRoles = []RuntimeRole{
	RoleRouter,
	RoleRaw,
	RoleCritical,
	RoleSummary,
	RoleSynthesisTooling,
	RoleSynthesisFinal,
}

type RoutedRequest struct {
	Role       RuntimeRole
	Messages   []Message
	Tools      []ToolDefinition
	ToolChoice string
}

type RoutedResult struct {
	GenerateResult
	Role     RuntimeRole
	Attempts []Attempt
}

type Attempt struct {
	Model     string
	Number    int
	ErrorCode ErrorCode
}

type RouterOptions struct {
	ProviderFailureThreshold int
	CircuitOpenDuration      time.Duration
	Now                      func() time.Time
}

type ModelRouter struct {
	provider ModelProvider
	policies map[RuntimeRole]config.RoleModelConfig
	breaker  *providerCircuitBreaker
}

func NewModelRouter(
	modelProvider ModelProvider,
	roles map[string]config.RoleModelConfig,
	models map[string]config.ModelDefinition,
	options RouterOptions,
) (*ModelRouter, error) {
	if modelProvider == nil {
		return nil, errors.New("model provider is required")
	}
	policies := make(map[RuntimeRole]config.RoleModelConfig, len(ordinaryRuntimeRoles))
	for _, role := range ordinaryRuntimeRoles {
		policy, ok := roles[string(role)]
		if !ok {
			return nil, fmt.Errorf("model policy for role %s is required", role)
		}
		if err := validateRolePolicy(role, policy, models); err != nil {
			return nil, err
		}
		policy.Fallback = append([]string(nil), policy.Fallback...)
		policies[role] = policy
	}
	return &ModelRouter{
		provider: modelProvider,
		policies: policies,
		breaker:  newProviderCircuitBreaker(options),
	}, nil
}

func (r *ModelRouter) Generate(ctx context.Context, request RoutedRequest) (RoutedResult, error) {
	if err := ctx.Err(); err != nil {
		return RoutedResult{Role: request.Role}, err
	}
	policy, ok := r.policies[request.Role]
	if !ok {
		return RoutedResult{Role: request.Role}, fmt.Errorf("unknown runtime role %q", request.Role)
	}
	if allowed, code := r.breaker.allow(); !allowed {
		return RoutedResult{Role: request.Role}, normalizedError(code, "", 0, errors.New("provider circuit is open"))
	}

	models := make([]string, 0, len(policy.Fallback)+1)
	models = append(models, policy.Primary)
	models = append(models, policy.Fallback...)
	result := RoutedResult{Role: request.Role}
	var lastErr error

	for _, model := range models {
		for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			callCtx := ctx
			cancel := func() {}
			if timeout := policy.Timeout.Value(); timeout > 0 {
				callCtx, cancel = context.WithTimeout(ctx, timeout)
			}
			generated, err := r.provider.Generate(callCtx, GenerateRequest{
				Model:           model,
				Messages:        request.Messages,
				Tools:           request.Tools,
				ToolChoice:      request.ToolChoice,
				Temperature:     policy.Temperature,
				MaxOutputTokens: policy.MaxOutputTokens,
			})
			cancel()

			attemptRecord := Attempt{Model: model, Number: attempt + 1}
			if err == nil {
				result.GenerateResult = generated
				result.Attempts = append(result.Attempts, attemptRecord)
				r.breaker.success()
				return result, nil
			}
			if ctx.Err() != nil {
				r.breaker.abortProbe()
				return result, ctx.Err()
			}
			lastErr = err
			if code, exists := ErrorCodeOf(err); exists {
				attemptRecord.ErrorCode = code
			}
			result.Attempts = append(result.Attempts, attemptRecord)

			code, normalized := ErrorCodeOf(err)
			if normalized && code == CodeProviderExhausted {
				r.breaker.failure(code)
				return result, err
			}
			if normalized && code == CodeProviderUnavailable {
				r.breaker.failure(code)
				return result, err
			}
			if normalized {
				r.breaker.success()
			}
			if !IsFallbackEligible(err) {
				if !normalized {
					r.breaker.abortProbe()
				}
				return result, err
			}
			if normalized && code == CodeModelRateLimited {
				break
			}
			if attempt < policy.MaxRetries {
				continue
			}
			break
		}
	}
	if lastErr == nil {
		lastErr = normalizedError(CodeProviderUnavailable, "", 0, errors.New("no model attempts were made"))
	}
	return result, lastErr
}

func validateRolePolicy(role RuntimeRole, policy config.RoleModelConfig, models map[string]config.ModelDefinition) error {
	if policy.Primary == "" {
		return fmt.Errorf("primary model for role %s is required", role)
	}
	if policy.MaxRetries < 0 {
		return fmt.Errorf("retry budget for role %s cannot be negative", role)
	}
	if policy.Timeout.Value() <= 0 {
		return fmt.Errorf("timeout for role %s must be positive", role)
	}
	if policy.MaxOutputTokens <= 0 {
		return fmt.Errorf("output limit for role %s must be positive", role)
	}
	seen := make(map[string]struct{}, len(policy.Fallback)+1)
	for _, modelID := range append([]string{policy.Primary}, policy.Fallback...) {
		if _, duplicate := seen[modelID]; duplicate {
			return fmt.Errorf("role %s contains duplicate model %s", role, modelID)
		}
		seen[modelID] = struct{}{}
		model, ok := models[modelID]
		if !ok || !model.Enabled {
			return fmt.Errorf("role %s references disabled model %s", role, modelID)
		}
		if model.ProviderRef != "open_code_zen" || model.Protocol != "openai_chat_completions" {
			return fmt.Errorf("role %s references incompatible model %s", role, modelID)
		}
		if model.ReservedFor != "" {
			return fmt.Errorf("role %s cannot use reserved model %s", role, modelID)
		}
	}
	return nil
}

type providerCircuitBreaker struct {
	mu        sync.Mutex
	threshold int
	openFor   time.Duration
	now       func() time.Time

	failures  int
	openUntil time.Time
	openCode  ErrorCode
	halfOpen  bool
}

func newProviderCircuitBreaker(options RouterOptions) *providerCircuitBreaker {
	threshold := options.ProviderFailureThreshold
	if threshold <= 0 {
		threshold = 3
	}
	openFor := options.CircuitOpenDuration
	if openFor <= 0 {
		openFor = 30 * time.Second
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &providerCircuitBreaker{threshold: threshold, openFor: openFor, now: now}
}

func (b *providerCircuitBreaker) allow() (bool, ErrorCode) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return true, ""
	}
	if b.now().Before(b.openUntil) {
		return false, b.openCode
	}
	if b.halfOpen {
		return false, b.openCode
	}
	b.halfOpen = true
	return true, ""
}

func (b *providerCircuitBreaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openUntil = time.Time{}
	b.openCode = ""
	b.halfOpen = false
}

func (b *providerCircuitBreaker) abortProbe() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.halfOpen {
		b.halfOpen = false
	}
}

func (b *providerCircuitBreaker) failure(code ErrorCode) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if code == CodeProviderExhausted {
		b.open(code)
		return
	}
	if code != CodeProviderUnavailable {
		return
	}
	b.failures++
	if b.halfOpen || b.failures >= b.threshold {
		b.open(code)
	}
}

func (b *providerCircuitBreaker) open(code ErrorCode) {
	b.openUntil = b.now().Add(b.openFor)
	b.openCode = code
	b.halfOpen = false
}
