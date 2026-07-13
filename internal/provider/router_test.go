package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

type providerFunc struct {
	mu       sync.Mutex
	generate func(context.Context, GenerateRequest) (GenerateResult, error)
	calls    []GenerateRequest
}

func (p *providerFunc) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	p.mu.Lock()
	p.calls = append(p.calls, request)
	fn := p.generate
	p.mu.Unlock()
	return fn(ctx, request)
}

func (p *providerFunc) Stream(context.Context, GenerateRequest) (<-chan StreamEvent, error) {
	return nil, errors.New("not implemented in router tests")
}

func (p *providerFunc) ListModels(context.Context) ([]ModelDescriptor, error) {
	return nil, errors.New("not implemented in router tests")
}

func (p *providerFunc) modelsCalled() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	models := make([]string, len(p.calls))
	for index, call := range p.calls {
		models[index] = call.Model
	}
	return models
}

func TestRoleRouterUsesAcceptedRoleFallbackOrder(t *testing.T) {
	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		if request.Model == "deepseek-v4-flash-free" {
			return GenerateResult{}, normalizedError(CodeModelUnavailable, request.Model, 404, nil)
		}
		return GenerateResult{Model: request.Model, Content: "ok"}, nil
	}
	router := newTestRoleRouter(t, upstream, defaultRolePolicies(), RouterOptions{})
	result, err := router.Generate(context.Background(), RoutedRequest{
		Role: RoleRaw, Messages: []Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Model != "mimo-v2.5-free" || result.Content != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertStrings(t, upstream.modelsCalled(), []string{"deepseek-v4-flash-free", "mimo-v2.5-free"})
	if len(result.Attempts) != 2 || result.Attempts[0].ErrorCode != CodeModelUnavailable {
		t.Fatalf("unexpected attempts: %#v", result.Attempts)
	}
}

func TestRoleRouterUsesSynthesisFallbackChain(t *testing.T) {
	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		if request.Model != "mimo-v2.5-free" {
			return GenerateResult{}, normalizedError(CodeModelResponseInvalid, request.Model, 200, nil)
		}
		return GenerateResult{Model: request.Model, Content: "final"}, nil
	}
	router := newTestRoleRouter(t, upstream, defaultRolePolicies(), RouterOptions{})
	result, err := router.Generate(context.Background(), RoutedRequest{
		Role: RoleSynthesisFinal, Messages: []Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Model != "mimo-v2.5-free" {
		t.Fatalf("model = %q", result.Model)
	}
	assertStrings(t, upstream.modelsCalled(), []string{"big-pickle", "deepseek-v4-flash-free", "mimo-v2.5-free"})
}

func TestRoleRouterHonorsRetryBudgetBeforeFallback(t *testing.T) {
	policies := defaultRolePolicies()
	policy := policies[string(RoleRaw)]
	policy.MaxRetries = 1
	policies[string(RoleRaw)] = policy

	var deepAttempts int
	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		if request.Model == "deepseek-v4-flash-free" {
			deepAttempts++
			if deepAttempts == 1 {
				return GenerateResult{}, normalizedError(CodeModelTimeout, request.Model, 0, nil)
			}
		}
		return GenerateResult{Model: request.Model, Content: "ok"}, nil
	}
	router := newTestRoleRouter(t, upstream, policies, RouterOptions{})
	result, err := router.Generate(context.Background(), RoutedRequest{
		Role: RoleRaw, Messages: []Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Model != "deepseek-v4-flash-free" {
		t.Fatalf("unexpected fallback after successful retry: %#v", result)
	}
	assertStrings(t, upstream.modelsCalled(), []string{"deepseek-v4-flash-free", "deepseek-v4-flash-free"})
}

func TestRoleRouterRateLimitFallsBackWithoutRetryingSameModel(t *testing.T) {
	policies := defaultRolePolicies()
	policy := policies[string(RoleRaw)]
	policy.MaxRetries = 2
	policies[string(RoleRaw)] = policy

	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		if request.Model == "deepseek-v4-flash-free" {
			return GenerateResult{}, normalizedError(CodeModelRateLimited, request.Model, 429, nil)
		}
		return GenerateResult{Model: request.Model, Content: "fallback"}, nil
	}
	router := newTestRoleRouter(t, upstream, policies, RouterOptions{})
	_, err := router.Generate(context.Background(), RoutedRequest{
		Role: RoleRaw, Messages: []Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	assertStrings(t, upstream.modelsCalled(), []string{"deepseek-v4-flash-free", "mimo-v2.5-free"})
}

func TestRoleRouterProviderExhaustionStopsFallbackAndOpensCircuit(t *testing.T) {
	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		return GenerateResult{}, normalizedError(CodeProviderExhausted, request.Model, 402, nil)
	}
	router := newTestRoleRouter(t, upstream, defaultRolePolicies(), RouterOptions{})
	request := RoutedRequest{Role: RoleSynthesisFinal, Messages: []Message{{Role: "user", Content: "question"}}}
	_, err := router.Generate(context.Background(), request)
	assertErrorCode(t, err, CodeProviderExhausted)
	_, err = router.Generate(context.Background(), request)
	assertErrorCode(t, err, CodeProviderExhausted)
	assertStrings(t, upstream.modelsCalled(), []string{"big-pickle"})
}

func TestRoleRouterCircuitHalfOpenProbeCanRecover(t *testing.T) {
	now := time.Unix(100, 0)
	exhausted := true
	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		if exhausted {
			return GenerateResult{}, normalizedError(CodeProviderExhausted, request.Model, 402, nil)
		}
		return GenerateResult{Model: request.Model, Content: "recovered"}, nil
	}
	router := newTestRoleRouter(t, upstream, defaultRolePolicies(), RouterOptions{
		CircuitOpenDuration: time.Minute,
		Now:                 func() time.Time { return now },
	})
	request := RoutedRequest{Role: RoleRouter, Messages: []Message{{Role: "user", Content: "hello"}}}
	_, err := router.Generate(context.Background(), request)
	assertErrorCode(t, err, CodeProviderExhausted)
	now = now.Add(2 * time.Minute)
	exhausted = false
	result, err := router.Generate(context.Background(), request)
	if err != nil || result.Content != "recovered" {
		t.Fatalf("half-open recovery result=%#v error=%v", result, err)
	}
	_, err = router.Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("closed circuit rejected request: %v", err)
	}
	assertStrings(t, upstream.modelsCalled(), []string{"nemotron-3-ultra-free", "nemotron-3-ultra-free", "nemotron-3-ultra-free"})
}

func TestRoleRouterProviderFailureThreshold(t *testing.T) {
	upstream := &providerFunc{}
	upstream.generate = func(_ context.Context, request GenerateRequest) (GenerateResult, error) {
		return GenerateResult{}, normalizedError(CodeProviderUnavailable, request.Model, 503, nil)
	}
	router := newTestRoleRouter(t, upstream, defaultRolePolicies(), RouterOptions{ProviderFailureThreshold: 2})
	request := RoutedRequest{Role: RoleSummary, Messages: []Message{{Role: "user", Content: "question"}}}
	for range 2 {
		_, err := router.Generate(context.Background(), request)
		assertErrorCode(t, err, CodeProviderUnavailable)
	}
	_, err := router.Generate(context.Background(), request)
	assertErrorCode(t, err, CodeProviderUnavailable)
	assertStrings(t, upstream.modelsCalled(), []string{"nemotron-3-ultra-free", "nemotron-3-ultra-free"})
}

func TestRoleRouterRejectsReservedModelInOrdinaryPipeline(t *testing.T) {
	policies := defaultRolePolicies()
	policy := policies[string(RoleRaw)]
	policy.Primary = "north-mini-code-free"
	policies[string(RoleRaw)] = policy
	_, err := NewModelRouter(&providerFunc{generate: func(context.Context, GenerateRequest) (GenerateResult, error) {
		return GenerateResult{}, nil
	}}, policies, defaultModelDefinitions(), RouterOptions{})
	if err == nil {
		t.Fatal("reserved code model was accepted for an ordinary runtime role")
	}
}

func newTestRoleRouter(t *testing.T, upstream ModelProvider, policies map[string]config.RoleModelConfig, options RouterOptions) *ModelRouter {
	t.Helper()
	router, err := NewModelRouter(upstream, policies, defaultModelDefinitions(), options)
	if err != nil {
		t.Fatalf("NewModelRouter() error = %v", err)
	}
	return router
}

func defaultRolePolicies() map[string]config.RoleModelConfig {
	role := func(primary string, fallback ...string) config.RoleModelConfig {
		return config.RoleModelConfig{
			Primary: primary, Fallback: fallback, Timeout: config.Duration(time.Second), MaxOutputTokens: 1024,
		}
	}
	return map[string]config.RoleModelConfig{
		string(RoleRouter):           role("nemotron-3-ultra-free", "mimo-v2.5-free"),
		string(RoleRaw):              role("deepseek-v4-flash-free", "mimo-v2.5-free"),
		string(RoleCritical):         role("deepseek-v4-flash-free", "mimo-v2.5-free"),
		string(RoleSummary):          role("nemotron-3-ultra-free", "mimo-v2.5-free"),
		string(RoleSynthesisTooling): role("big-pickle", "deepseek-v4-flash-free", "mimo-v2.5-free"),
		string(RoleSynthesisFinal):   role("big-pickle", "deepseek-v4-flash-free", "mimo-v2.5-free"),
	}
}

func defaultModelDefinitions() map[string]config.ModelDefinition {
	model := func() config.ModelDefinition {
		return config.ModelDefinition{ProviderRef: "open_code_zen", Protocol: "openai_chat_completions", Enabled: true}
	}
	models := map[string]config.ModelDefinition{
		"big-pickle":             model(),
		"mimo-v2.5-free":         model(),
		"nemotron-3-ultra-free":  model(),
		"deepseek-v4-flash-free": model(),
		"north-mini-code-free":   model(),
	}
	reserved := models["north-mini-code-free"]
	reserved.ReservedFor = "future_code_workflow"
	models["north-mini-code-free"] = reserved
	return models
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("values = %#v, want %#v", got, want)
		}
	}
}
