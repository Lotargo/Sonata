package application

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/database"
	"github.com/Lotargo/Sonata/internal/protected"
	"github.com/Lotargo/Sonata/internal/provider"
	"github.com/jackc/pgx/v5/pgtype"
)

type contextKey string

const manifestContentKey contextKey = "manifest_content"

// ContextWithUserManifests stores user manifest ID to content mapping in context.
func ContextWithUserManifests(ctx context.Context, manifests map[string]string) context.Context {
	return context.WithValue(ctx, manifestContentKey, manifests)
}

func UserManifestsFromContext(ctx context.Context) (map[string]string, bool) {
	manifests, ok := ctx.Value(manifestContentKey).(map[string]string)
	return manifests, ok
}

// UserManifestFromContext retrieves user manifest content by its ID.
func UserManifestFromContext(ctx context.Context, id string) (string, bool) {
	m, ok := ctx.Value(manifestContentKey).(map[string]string)
	if !ok {
		return "", false
	}
	content, ok := m[id]
	return content, ok
}

type runContextKey struct{}
type RunContextInfo struct {
	OwnerID        string
	CognitiveRunID pgtype.UUID
	RoleRunIDs     map[cognition.RuntimeRole]pgtype.UUID
}

func ContextWithRunInfo(ctx context.Context, info RunContextInfo) context.Context {
	return context.WithValue(ctx, runContextKey{}, info)
}

func RunInfoFromContext(ctx context.Context) (RunContextInfo, bool) {
	info, ok := ctx.Value(runContextKey{}).(RunContextInfo)
	return info, ok
}

type usageCollectorKey struct{}
type UsageCollector struct {
	mu    sync.Mutex
	Usage provider.Usage
	Model string
}

func (c *UsageCollector) Record(model string, u provider.Usage) {
	c.mu.Lock()
	c.Model = model
	c.Usage = u
	c.mu.Unlock()
}

// RunnerAdapter orchestrates system/user prompt compilation, calling the model provider via the router,
// stripping markdown blocks, and decoding the structured responses strictly.
type RunnerAdapter struct {
	router    *provider.ModelRouter
	compiler  *protected.PromptCompiler
	bundle    *protected.Bundle
	usageRepo *database.ProviderUsageRepository
}

// NewRunnerAdapter creates a new RunnerAdapter.
func NewRunnerAdapter(
	router *provider.ModelRouter,
	compiler *protected.PromptCompiler,
	bundle *protected.Bundle,
	usageRepo *database.ProviderUsageRepository,
) (*RunnerAdapter, error) {
	if router == nil {
		return nil, errors.New("model router is required")
	}
	if compiler == nil {
		return nil, errors.New("prompt compiler is required")
	}
	if bundle == nil {
		return nil, errors.New("protected bundle is required")
	}
	return &RunnerAdapter{
		router:    router,
		compiler:  compiler,
		bundle:    bundle,
		usageRepo: usageRepo,
	}, nil
}

func (r *RunnerAdapter) recordUsage(ctx context.Context, role cognition.RuntimeRole, model string, usage provider.Usage) {
	if r.usageRepo == nil {
		return
	}
	if info, ok := RunInfoFromContext(ctx); ok {
		_, _ = r.usageRepo.Insert(ctx, database.InsertProviderUsageInput{
			OwnerID:        info.OwnerID,
			CognitiveRunID: info.CognitiveRunID,
			RoleRunID:      info.RoleRunIDs[role],
			Provider:       "open_code_zen",
			ModelID:        model,
			InputTokens:    int64(usage.InputTokens),
			OutputTokens:   int64(usage.OutputTokens),
			CachedTokens:   int64(usage.CachedTokens),
			CreatedAt:      time.Now(),
		})
	}
}

// RunRouter implements cognition.RouterRunner.
func (r *RunnerAdapter) RunRouter(ctx context.Context, input cognition.RouterInput) (cognition.RouterRunResult, error) {
	if err := input.Validate(); err != nil {
		return cognition.RouterRunResult{}, err
	}

	defaultManifest, ok := r.bundle.DefaultManifests["manifest.router.default"]
	if !ok {
		return cognition.RouterRunResult{}, errors.New("default manifest for router not found")
	}
	resolvedManifest := protected.ResolvedManifest{
		Source:   protected.ManifestSourceSystemDefault,
		Metadata: defaultManifest.Metadata,
		Default:  &defaultManifest,
	}

	runtimeCtx := protected.RuntimeContext{
		UserInput: input.UserInput,
	}

	compiled, err := r.compiler.Compile(protected.CompileInput{
		InstructionID: "router",
		Manifest:      resolvedManifest,
		Runtime:       runtimeCtx,
	})
	if err != nil {
		return cognition.RouterRunResult{}, fmt.Errorf("compile router prompt: %w", err)
	}

	messages := make([]provider.Message, len(compiled.Messages))
	for i, m := range compiled.Messages {
		messages[i] = provider.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}
	var finalMessages []provider.Message
	if len(input.History) > 0 {
		finalMessages = append(finalMessages, messages[0]) // system
		for _, h := range input.History {
			finalMessages = append(finalMessages, provider.Message{
				Role:    h.Role,
				Content: h.Content,
			})
		}
		finalMessages = append(finalMessages, messages[1]) // user
	} else {
		finalMessages = messages
	}

	startTime := time.Now()
	res, err := r.router.Generate(ctx, provider.RoutedRequest{
		Role:     provider.RoleRouter,
		Messages: finalMessages,
	})
	latency := time.Since(startTime)
	if err != nil {
		return cognition.RouterRunResult{}, err
	}

	if collector, ok := ctx.Value(usageCollectorKey{}).(*UsageCollector); ok {
		collector.Record(res.Model, res.Usage)
	}

	trimmed := StripMarkdownJSON(res.Content)
	output, err := cognition.DecodeRouterOutput([]byte(trimmed))
	if err != nil {
		return cognition.RouterRunResult{}, fmt.Errorf("decode router output: %w", err)
	}

	return cognition.RouterRunResult{
		Output: output,
		Metadata: cognition.RoleMetadata{
			Role:    cognition.RoleRouter,
			Status:  cognition.RoleStatusSucceeded,
			ModelID: res.Model,
			Latency: latency,
			Instruction: cognition.ArtifactRef{
				ID:      compiled.Metadata.Instruction.ID,
				Version: compiled.Metadata.Instruction.Version,
				Hash:    compiled.Metadata.Instruction.Hash,
			},
			Manifest: cognition.ManifestRef{
				ArtifactRef: cognition.ArtifactRef{
					ID:      compiled.Metadata.Manifest.ID,
					Version: compiled.Metadata.Manifest.Version,
					Hash:    compiled.Metadata.Manifest.Hash,
				},
				Source: string(compiled.Metadata.ManifestSource),
			},
		},
	}, nil
}

// RunRaw implements cognition.RawRunner.
func (r *RunnerAdapter) RunRaw(ctx context.Context, input cognition.RawInput) (cognition.PrismReport, error) {
	role := cognition.RuntimeRole(string(input.Prism) + "_raw")

	resolvedManifest, err := r.resolveManifest(ctx, input.Manifest)
	if err != nil {
		return cognition.PrismReport{}, err
	}

	runtimeCtx := protected.RuntimeContext{
		UserInput:     input.UserInput,
		EmotionReport: input.Emotion.Text,
		ContextPack:   input.Context.Text,
	}

	compiled, err := r.compiler.Compile(protected.CompileInput{
		InstructionID: input.Instruction.ID,
		Manifest:      resolvedManifest,
		Runtime:       runtimeCtx,
	})
	if err != nil {
		return cognition.PrismReport{}, fmt.Errorf("compile raw prompt: %w", err)
	}

	messages := make([]provider.Message, len(compiled.Messages))
	for i, m := range compiled.Messages {
		messages[i] = provider.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}
	var finalMessages []provider.Message
	if len(input.History) > 0 {
		finalMessages = append(finalMessages, messages[0]) // system
		for _, h := range input.History {
			finalMessages = append(finalMessages, provider.Message{
				Role:    h.Role,
				Content: h.Content,
			})
		}
		finalMessages = append(finalMessages, messages[1]) // user
	} else {
		finalMessages = messages
	}

	startTime := time.Now()
	res, err := r.router.Generate(ctx, provider.RoutedRequest{
		Role:     provider.RoleRaw,
		Messages: finalMessages,
	})
	latency := time.Since(startTime)
	if err != nil {
		return cognition.PrismReport{}, err
	}
	r.recordUsage(ctx, cognition.RuntimeRole(string(input.Prism)+"_raw"), res.Model, res.Usage)

	trimmed := StripMarkdownJSON(res.Content)
	content, confidence, err := DecodePrismReport([]byte(trimmed))
	if err != nil {
		return cognition.PrismReport{}, fmt.Errorf("decode raw report: %w", err)
	}

	return cognition.PrismReport{
		Prism:      input.Prism,
		Content:    content,
		Confidence: confidence,
		Metadata: cognition.RoleMetadata{
			Role:        role,
			Status:      cognition.RoleStatusSucceeded,
			ModelID:     res.Model,
			Latency:     latency,
			Instruction: input.Instruction,
			Manifest:    input.Manifest,
		},
	}, nil
}

// RunCritical implements cognition.CriticalRunner.
func (r *RunnerAdapter) RunCritical(ctx context.Context, input cognition.CriticalInput) (cognition.CriticalReport, error) {
	role := cognition.RuntimeRole(string(input.Prism) + "_critical")

	resolvedManifest, err := r.resolveManifest(ctx, input.Manifest)
	if err != nil {
		return cognition.CriticalReport{}, err
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf(`<raw-report confidence="%.2f">`, input.Raw.Confidence))
	_ = xml.EscapeText(&buf, []byte(input.Raw.Content))
	buf.WriteString(`</raw-report>`)

	runtimeCtx := protected.RuntimeContext{
		UserInput:     input.UserInput,
		EmotionReport: input.Emotion.Text,
		ContextPack:   input.Context.Text,
		RoleInput:     buf.String(),
	}

	compiled, err := r.compiler.Compile(protected.CompileInput{
		InstructionID: input.Instruction.ID,
		Manifest:      resolvedManifest,
		Runtime:       runtimeCtx,
	})
	if err != nil {
		return cognition.CriticalReport{}, fmt.Errorf("compile critical prompt: %w", err)
	}

	messages := make([]provider.Message, len(compiled.Messages))
	for i, m := range compiled.Messages {
		messages[i] = provider.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	startTime := time.Now()
	res, err := r.router.Generate(ctx, provider.RoutedRequest{
		Role:     provider.RoleCritical,
		Messages: messages,
	})
	latency := time.Since(startTime)
	if err != nil {
		return cognition.CriticalReport{}, err
	}
	r.recordUsage(ctx, cognition.RuntimeRole(string(input.Prism)+"_critical"), res.Model, res.Usage)

	trimmed := StripMarkdownJSON(res.Content)
	content, weakAssumptions, unprovenConclusions, confidence, err := DecodeCriticalReport([]byte(trimmed))
	if err != nil {
		return cognition.CriticalReport{}, fmt.Errorf("decode critical report: %w", err)
	}

	return cognition.CriticalReport{
		Prism:               input.Prism,
		Content:             content,
		WeakAssumptions:     weakAssumptions,
		UnprovenConclusions: unprovenConclusions,
		Confidence:          confidence,
		Metadata: cognition.RoleMetadata{
			Role:        role,
			Status:      cognition.RoleStatusSucceeded,
			ModelID:     res.Model,
			Latency:     latency,
			Instruction: input.Instruction,
			Manifest:    input.Manifest,
		},
	}, nil
}

// RunSummary implements cognition.SummaryRunner.
func (r *RunnerAdapter) RunSummary(ctx context.Context, input cognition.SummaryInput) (cognition.PrismSummary, error) {
	role := cognition.RuntimeRole(string(input.Prism) + "_summary")

	resolvedManifest, err := r.resolveManifest(ctx, input.Manifest)
	if err != nil {
		return cognition.PrismSummary{}, err
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf(`<raw-report confidence="%.2f">`, input.Raw.Confidence))
	_ = xml.EscapeText(&buf, []byte(input.Raw.Content))
	buf.WriteString(`</raw-report>`)
	buf.WriteString(fmt.Sprintf(`<critical-report confidence="%.2f">`, input.Critical.Confidence))
	_ = xml.EscapeText(&buf, []byte(input.Critical.Content))
	buf.WriteString(`</critical-report>`)

	runtimeCtx := protected.RuntimeContext{
		RoleInput: buf.String(),
	}

	compiled, err := r.compiler.Compile(protected.CompileInput{
		InstructionID: input.Instruction.ID,
		Manifest:      resolvedManifest,
		Runtime:       runtimeCtx,
	})
	if err != nil {
		return cognition.PrismSummary{}, fmt.Errorf("compile summary prompt: %w", err)
	}

	messages := make([]provider.Message, len(compiled.Messages))
	for i, m := range compiled.Messages {
		messages[i] = provider.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	startTime := time.Now()
	res, err := r.router.Generate(ctx, provider.RoutedRequest{
		Role:     provider.RoleSummary,
		Messages: messages,
	})
	latency := time.Since(startTime)
	if err != nil {
		return cognition.PrismSummary{}, err
	}
	r.recordUsage(ctx, cognition.RuntimeRole(string(input.Prism)+"_summary"), res.Model, res.Usage)

	trimmed := StripMarkdownJSON(res.Content)
	initialPos, mainCritique, revisedPos, rejectedAssumptions, openQuestions, confidence, err := DecodePrismSummary([]byte(trimmed))
	if err != nil {
		return cognition.PrismSummary{}, fmt.Errorf("decode prism summary: %w", err)
	}

	return cognition.PrismSummary{
		Prism:               input.Prism,
		InitialPosition:     initialPos,
		MainCritique:        mainCritique,
		RevisedPosition:     revisedPos,
		RejectedAssumptions: rejectedAssumptions,
		OpenQuestions:       openQuestions,
		Confidence:          confidence,
		Metadata: cognition.RoleMetadata{
			Role:        role,
			Status:      cognition.RoleStatusSucceeded,
			ModelID:     res.Model,
			Latency:     latency,
			Instruction: input.Instruction,
			Manifest:    input.Manifest,
		},
	}, nil
}

// RunSynthesisTooling implements cognition.SynthesisToolingRunner.
func (r *RunnerAdapter) RunSynthesisTooling(ctx context.Context, input cognition.SynthesisToolingInput) (cognition.SynthesisToolingOutput, error) {
	resolvedManifest, err := r.resolveManifest(ctx, input.Manifest)
	if err != nil {
		return cognition.SynthesisToolingOutput{}, err
	}

	runtimeCtx := protected.RuntimeContext{
		UserInput:        input.UserInput,
		EmotionReport:    input.Emotion.Text,
		ContextPack:      input.Context.Text,
		InternalDialogue: SerializeInternalDialogue(input.Dialogue),
	}

	compiled, err := r.compiler.Compile(protected.CompileInput{
		InstructionID: input.Instruction.ID,
		Manifest:      resolvedManifest,
		Runtime:       runtimeCtx,
	})
	if err != nil {
		return cognition.SynthesisToolingOutput{}, fmt.Errorf("compile synthesis tooling prompt: %w", err)
	}

	messages := make([]provider.Message, len(compiled.Messages))
	for i, m := range compiled.Messages {
		messages[i] = provider.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}
	var finalMessages []provider.Message
	if len(input.History) > 0 {
		finalMessages = append(finalMessages, messages[0]) // system
		for _, h := range input.History {
			finalMessages = append(finalMessages, provider.Message{
				Role:    h.Role,
				Content: h.Content,
			})
		}
		finalMessages = append(finalMessages, messages[1]) // user
	} else {
		finalMessages = messages
	}

	tools := []provider.ToolDefinition{
		{
			Type: "function",
			Function: provider.FunctionDefinition{
				Name:        "web.search.langsearch",
				Description: "Search the web using LangSearch.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"q": map[string]any{
							"type":        "string",
							"description": "The search query to execute.",
						},
					},
					"required": []string{"q"},
				},
			},
		},
		{
			Type: "function",
			Function: provider.FunctionDefinition{
				Name:        "memory.search.additional",
				Description: "Search user memory for additional context.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"q": map[string]any{
							"type":        "string",
							"description": "The search query to execute.",
						},
					},
					"required": []string{"q"},
				},
			},
		},
	}

	startTime := time.Now()
	res, err := r.router.Generate(ctx, provider.RoutedRequest{
		Role:       provider.RoleSynthesisTooling,
		Messages:   finalMessages,
		Tools:      tools,
		ToolChoice: "auto",
	})
	latency := time.Since(startTime)
	if err != nil {
		return cognition.SynthesisToolingOutput{}, err
	}
	r.recordUsage(ctx, cognition.RoleSynthesisTooling, res.Model, res.Usage)

	trimmed := StripMarkdownJSON(res.Content)
	preliminaryDecision, toolCalls, err := DecodeSynthesisToolingOutput([]byte(trimmed))
	if err != nil {
		if len(res.ToolCalls) > 0 {
			preliminaryDecision = res.Content
			if preliminaryDecision == "" {
				preliminaryDecision = "Calling tools..."
			}
			err = nil
		} else {
			return cognition.SynthesisToolingOutput{}, fmt.Errorf("decode synthesis tooling output: %w", err)
		}
	}

	if len(toolCalls) == 0 && len(res.ToolCalls) > 0 {
		toolCalls = make([]cognition.ToolCall, len(res.ToolCalls))
		for i, tc := range res.ToolCalls {
			toolCalls[i] = cognition.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return cognition.SynthesisToolingOutput{
		PreliminaryDecision: preliminaryDecision,
		ToolCalls:           toolCalls,
		Metadata: cognition.RoleMetadata{
			Role:        cognition.RoleSynthesisTooling,
			Status:      cognition.RoleStatusSucceeded,
			ModelID:     res.Model,
			Latency:     latency,
			Instruction: input.Instruction,
			Manifest:    input.Manifest,
		},
	}, nil
}

// RunSynthesisFinal implements cognition.SynthesisFinalRunner.
func (r *RunnerAdapter) RunSynthesisFinal(ctx context.Context, input cognition.SynthesisFinalInput) (cognition.SynthesisFinalOutput, error) {
	resolvedManifest, err := r.resolveManifest(ctx, input.Manifest)
	if err != nil {
		return cognition.SynthesisFinalOutput{}, err
	}

	runtimeCtx := protected.RuntimeContext{
		UserInput:        input.UserInput,
		EmotionReport:    input.Emotion.Text,
		ContextPack:      input.Context.Text,
		InternalDialogue: SerializeInternalDialogue(input.Dialogue),
		RoleInput:        input.PreliminaryDecision,
		ToolResults:      SerializeToolResults(input.ToolResults),
	}

	compiled, err := r.compiler.Compile(protected.CompileInput{
		InstructionID: input.Instruction.ID,
		Manifest:      resolvedManifest,
		Runtime:       runtimeCtx,
	})
	if err != nil {
		return cognition.SynthesisFinalOutput{}, fmt.Errorf("compile synthesis final prompt: %w", err)
	}

	messages := make([]provider.Message, len(compiled.Messages))
	for i, m := range compiled.Messages {
		messages[i] = provider.Message{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}
	var finalMessages []provider.Message
	if len(input.History) > 0 {
		finalMessages = append(finalMessages, messages[0]) // system
		for _, h := range input.History {
			finalMessages = append(finalMessages, provider.Message{
				Role:    h.Role,
				Content: h.Content,
			})
		}
		finalMessages = append(finalMessages, messages[1]) // user
	} else {
		finalMessages = messages
	}

	startTime := time.Now()
	res, err := r.router.Generate(ctx, provider.RoutedRequest{
		Role:     provider.RoleSynthesisFinal,
		Messages: finalMessages,
	})
	latency := time.Since(startTime)
	if err != nil {
		return cognition.SynthesisFinalOutput{}, err
	}
	r.recordUsage(ctx, cognition.RoleSynthesisFinal, res.Model, res.Usage)

	return cognition.SynthesisFinalOutput{
		Content: res.Content,
		Metadata: cognition.RoleMetadata{
			Role:        cognition.RoleSynthesisFinal,
			Status:      cognition.RoleStatusSucceeded,
			ModelID:     res.Model,
			Latency:     latency,
			Instruction: input.Instruction,
			Manifest:    input.Manifest,
		},
	}, nil
}

func (r *RunnerAdapter) resolveManifest(ctx context.Context, ref cognition.ManifestRef) (protected.ResolvedManifest, error) {
	if ref.Source == string(protected.ManifestSourceSystemDefault) {
		defaultManifest, ok := r.bundle.DefaultManifests[ref.ID]
		if !ok {
			return protected.ResolvedManifest{}, fmt.Errorf("default manifest %s not found", ref.ID)
		}
		return protected.ResolvedManifest{
			Source:   protected.ManifestSourceSystemDefault,
			Metadata: defaultManifest.Metadata,
			Default:  &defaultManifest,
		}, nil
	}

	content, ok := UserManifestFromContext(ctx, ref.ID)
	if !ok {
		return protected.ResolvedManifest{}, fmt.Errorf("user manifest content for %s not found in context", ref.ID)
	}

	return protected.ResolvedManifest{
		Source:   protected.ManifestSource(ref.Source),
		Metadata: protected.Metadata{
			ID:      ref.ID,
			Version: ref.Version,
			Hash:    ref.Hash,
		},
		UserText: content,
	}, nil
}

// StripMarkdownJSON removes ```json and ``` code blocks.
func StripMarkdownJSON(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "```json"); idx != -1 {
		content := s[idx+7:]
		if closeIdx := strings.Index(content, "```"); closeIdx != -1 {
			return strings.TrimSpace(content[:closeIdx])
		}
		return strings.TrimSpace(content)
	}
	if idx := strings.Index(s, "```"); idx != -1 {
		content := s[idx+3:]
		if closeIdx := strings.Index(content, "```"); closeIdx != -1 {
			return strings.TrimSpace(content[:closeIdx])
		}
		return strings.TrimSpace(content)
	}
	return s
}

// DecodePrismReport strictly decodes PrismReport from JSON.
func DecodePrismReport(data []byte) (content string, confidence float64, err error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return "", 0, err
	}
	delim, ok := start.(json.Delim)
	if !ok || delim != '{' {
		return "", 0, errors.New("raw report must be a JSON object")
	}

	seenContent := false
	seenConfidence := false

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", 0, err
		}
		key, ok := token.(string)
		if !ok {
			return "", 0, errors.New("raw report contains invalid field name")
		}

		switch key {
		case "content":
			if seenContent {
				return "", 0, errors.New("raw report repeats content")
			}
			var val string
			if err := decoder.Decode(&val); err != nil {
				return "", 0, err
			}
			content = val
			seenContent = true
		case "confidence":
			if seenConfidence {
				return "", 0, errors.New("raw report repeats confidence")
			}
			var val float64
			if err := decoder.Decode(&val); err != nil {
				return "", 0, err
			}
			confidence = val
			seenConfidence = true
		default:
			return "", 0, fmt.Errorf("raw report contains unknown field %q", key)
		}
	}

	end, err := decoder.Token()
	if err != nil {
		return "", 0, err
	}
	delim, ok = end.(json.Delim)
	if !ok || delim != '}' {
		return "", 0, errors.New("raw report has an invalid object boundary")
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", 0, errors.New("raw report must contain one JSON object")
		}
		return "", 0, err
	}

	if !seenContent {
		return "", 0, errors.New("raw report is missing content")
	}
	if !seenConfidence {
		return "", 0, errors.New("raw report is missing confidence")
	}

	return content, confidence, nil
}

// DecodeCriticalReport strictly decodes CriticalReport from JSON.
func DecodeCriticalReport(data []byte) (content string, weakAssumptions []string, unprovenConclusions []string, confidence float64, err error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return "", nil, nil, 0, err
	}
	delim, ok := start.(json.Delim)
	if !ok || delim != '{' {
		return "", nil, nil, 0, errors.New("critical report must be a JSON object")
	}

	seenContent := false
	seenWeakAssumptions := false
	seenUnprovenConclusions := false
	seenConfidence := false

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", nil, nil, 0, err
		}
		key, ok := token.(string)
		if !ok {
			return "", nil, nil, 0, errors.New("critical report contains invalid field name")
		}

		switch key {
		case "content":
			if seenContent {
				return "", nil, nil, 0, errors.New("critical report repeats content")
			}
			var val string
			if err := decoder.Decode(&val); err != nil {
				return "", nil, nil, 0, err
			}
			content = val
			seenContent = true
		case "weak_assumptions":
			if seenWeakAssumptions {
				return "", nil, nil, 0, errors.New("critical report repeats weak_assumptions")
			}
			var val []string
			if err := decoder.Decode(&val); err != nil {
				return "", nil, nil, 0, err
			}
			weakAssumptions = val
			seenWeakAssumptions = true
		case "unproven_conclusions":
			if seenUnprovenConclusions {
				return "", nil, nil, 0, errors.New("critical report repeats unproven_conclusions")
			}
			var val []string
			if err := decoder.Decode(&val); err != nil {
				return "", nil, nil, 0, err
			}
			unprovenConclusions = val
			seenUnprovenConclusions = true
		case "confidence":
			if seenConfidence {
				return "", nil, nil, 0, errors.New("critical report repeats confidence")
			}
			var val float64
			if err := decoder.Decode(&val); err != nil {
				return "", nil, nil, 0, err
			}
			confidence = val
			seenConfidence = true
		default:
			return "", nil, nil, 0, fmt.Errorf("critical report contains unknown field %q", key)
		}
	}

	end, err := decoder.Token()
	if err != nil {
		return "", nil, nil, 0, err
	}
	delim, ok = end.(json.Delim)
	if !ok || delim != '}' {
		return "", nil, nil, 0, errors.New("critical report has an invalid object boundary")
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", nil, nil, 0, errors.New("critical report must contain one JSON object")
		}
		return "", nil, nil, 0, err
	}

	if !seenContent {
		return "", nil, nil, 0, errors.New("critical report is missing content")
	}
	if !seenConfidence {
		return "", nil, nil, 0, errors.New("critical report is missing confidence")
	}
	if weakAssumptions == nil {
		weakAssumptions = []string{}
	}
	if unprovenConclusions == nil {
		unprovenConclusions = []string{}
	}

	return content, weakAssumptions, unprovenConclusions, confidence, nil
}

// DecodePrismSummary strictly decodes PrismSummary from JSON.
func DecodePrismSummary(data []byte) (initialPosition string, mainCritique string, revisedPosition string, rejectedAssumptions []string, openQuestions []string, confidence float64, err error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return "", "", "", nil, nil, 0, err
	}
	delim, ok := start.(json.Delim)
	if !ok || delim != '{' {
		return "", "", "", nil, nil, 0, errors.New("prism summary must be a JSON object")
	}

	seenInitial := false
	seenCritique := false
	seenRevised := false
	seenRejected := false
	seenQuestions := false
	seenConfidence := false

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", "", "", nil, nil, 0, err
		}
		key, ok := token.(string)
		if !ok {
			return "", "", "", nil, nil, 0, errors.New("prism summary contains invalid field name")
		}

		switch key {
		case "initial_position":
			if seenInitial {
				return "", "", "", nil, nil, 0, errors.New("prism summary repeats initial_position")
			}
			var val string
			if err := decoder.Decode(&val); err != nil {
				return "", "", "", nil, nil, 0, err
			}
			initialPosition = val
			seenInitial = true
		case "main_critique":
			if seenCritique {
				return "", "", "", nil, nil, 0, errors.New("prism summary repeats main_critique")
			}
			var val string
			if err := decoder.Decode(&val); err != nil {
				return "", "", "", nil, nil, 0, err
			}
			mainCritique = val
			seenCritique = true
		case "revised_position":
			if seenRevised {
				return "", "", "", nil, nil, 0, errors.New("prism summary repeats revised_position")
			}
			var val string
			if err := decoder.Decode(&val); err != nil {
				return "", "", "", nil, nil, 0, err
			}
			revisedPosition = val
			seenRevised = true
		case "rejected_assumptions":
			if seenRejected {
				return "", "", "", nil, nil, 0, errors.New("prism summary repeats rejected_assumptions")
			}
			var val []string
			if err := decoder.Decode(&val); err != nil {
				return "", "", "", nil, nil, 0, err
			}
			rejectedAssumptions = val
			seenRejected = true
		case "open_questions":
			if seenQuestions {
				return "", "", "", nil, nil, 0, errors.New("prism summary repeats open_questions")
			}
			var val []string
			if err := decoder.Decode(&val); err != nil {
				return "", "", "", nil, nil, 0, err
			}
			openQuestions = val
			seenQuestions = true
		case "confidence":
			if seenConfidence {
				return "", "", "", nil, nil, 0, errors.New("prism summary repeats confidence")
			}
			var val float64
			if err := decoder.Decode(&val); err != nil {
				return "", "", "", nil, nil, 0, err
			}
			confidence = val
			seenConfidence = true
		default:
			return "", "", "", nil, nil, 0, fmt.Errorf("prism summary contains unknown field %q", key)
		}
	}

	end, err := decoder.Token()
	if err != nil {
		return "", "", "", nil, nil, 0, err
	}
	delim, ok = end.(json.Delim)
	if !ok || delim != '}' {
		return "", "", "", nil, nil, 0, errors.New("prism summary has an invalid object boundary")
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(io.EOF, err) {
		if err == nil {
			return "", "", "", nil, nil, 0, errors.New("prism summary must contain one JSON object")
		}
		return "", "", "", nil, nil, 0, err
	}

	if !seenInitial {
		return "", "", "", nil, nil, 0, errors.New("prism summary is missing initial_position")
	}
	if !seenCritique {
		return "", "", "", nil, nil, 0, errors.New("prism summary is missing main_critique")
	}
	if !seenRevised {
		return "", "", "", nil, nil, 0, errors.New("prism summary is missing revised_position")
	}
	if !seenConfidence {
		return "", "", "", nil, nil, 0, errors.New("prism summary is missing confidence")
	}
	if rejectedAssumptions == nil {
		rejectedAssumptions = []string{}
	}
	if openQuestions == nil {
		openQuestions = []string{}
	}

	return initialPosition, mainCritique, revisedPosition, rejectedAssumptions, openQuestions, confidence, nil
}

// DecodeSynthesisToolingOutput strictly decodes SynthesisToolingOutput from JSON.
func DecodeSynthesisToolingOutput(data []byte) (preliminaryDecision string, toolCalls []cognition.ToolCall, err error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return "", nil, err
	}
	delim, ok := start.(json.Delim)
	if !ok || delim != '{' {
		return "", nil, errors.New("synthesis tooling output must be a JSON object")
	}

	seenDecision := false
	seenToolCalls := false

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", nil, err
		}
		key, ok := token.(string)
		if !ok {
			return "", nil, errors.New("synthesis tooling output contains invalid field name")
		}

		switch key {
		case "preliminary_decision":
			if seenDecision {
				return "", nil, errors.New("synthesis tooling output repeats preliminary_decision")
			}
			var val string
			if err := decoder.Decode(&val); err != nil {
				return "", nil, err
			}
			preliminaryDecision = val
			seenDecision = true
		case "tool_calls":
			if seenToolCalls {
				return "", nil, errors.New("synthesis tooling output repeats tool_calls")
			}
			var calls []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Arguments any    `json:"arguments"`
			}
			if err := decoder.Decode(&calls); err != nil {
				return "", nil, err
			}
			toolCalls = make([]cognition.ToolCall, len(calls))
			for i, c := range calls {
				argsStr := ""
				if c.Arguments != nil {
					switch a := c.Arguments.(type) {
					case string:
						argsStr = a
					default:
						b, err := json.Marshal(a)
						if err != nil {
							return "", nil, fmt.Errorf("marshal tool arguments: %w", err)
						}
						argsStr = string(b)
					}
				}
				toolCalls[i] = cognition.ToolCall{
					ID:        c.ID,
					Name:      c.Name,
					Arguments: argsStr,
				}
			}
			seenToolCalls = true
		default:
			return "", nil, fmt.Errorf("synthesis tooling output contains unknown field %q", key)
		}
	}

	end, err := decoder.Token()
	if err != nil {
		return "", nil, err
	}
	delim, ok = end.(json.Delim)
	if !ok || delim != '}' {
		return "", nil, errors.New("synthesis tooling output has an invalid object boundary")
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", nil, errors.New("synthesis tooling output must contain one JSON object")
		}
		return "", nil, err
	}

	if !seenDecision {
		return "", nil, errors.New("synthesis tooling output is missing preliminary_decision")
	}
	if toolCalls == nil {
		toolCalls = []cognition.ToolCall{}
	}

	return preliminaryDecision, toolCalls, nil
}

// SerializeInternalDialogue serializes cognition.InternalDialogue to XML.
func SerializeInternalDialogue(dialogue cognition.InternalDialogue) string {
	var buffer bytes.Buffer
	prisms := cognition.AllPrisms()
	for _, prism := range prisms {
		branch, exists := dialogue.Branches[prism]
		if !exists {
			continue
		}
		buffer.WriteString(fmt.Sprintf(`<prism id="%s">`, prism))

		buffer.WriteString(fmt.Sprintf(`<raw confidence="%.2f">`, branch.Raw.Confidence))
		_ = xml.EscapeText(&buffer, []byte(branch.Raw.Content))
		buffer.WriteString(`</raw>`)

		buffer.WriteString(fmt.Sprintf(`<critical confidence="%.2f">`, branch.Critical.Confidence))
		if len(branch.Critical.WeakAssumptions) > 0 {
			buffer.WriteString(`<weak-assumptions>`)
			for _, wa := range branch.Critical.WeakAssumptions {
				buffer.WriteString(`<assumption>`)
				_ = xml.EscapeText(&buffer, []byte(wa))
				buffer.WriteString(`</assumption>`)
			}
			buffer.WriteString(`</weak-assumptions>`)
		}
		if len(branch.Critical.UnprovenConclusions) > 0 {
			buffer.WriteString(`<unproven-conclusions>`)
			for _, uc := range branch.Critical.UnprovenConclusions {
				buffer.WriteString(`<conclusion>`)
				_ = xml.EscapeText(&buffer, []byte(uc))
				buffer.WriteString(`</conclusion>`)
			}
			buffer.WriteString(`</unproven-conclusions>`)
		}
		buffer.WriteString(`<content>`)
		_ = xml.EscapeText(&buffer, []byte(branch.Critical.Content))
		buffer.WriteString(`</content>`)
		buffer.WriteString(`</critical>`)

		buffer.WriteString(fmt.Sprintf(`<summary confidence="%.2f">`, branch.Summary.Confidence))
		buffer.WriteString(`<initial-position>`)
		_ = xml.EscapeText(&buffer, []byte(branch.Summary.InitialPosition))
		buffer.WriteString(`</initial-position>`)
		buffer.WriteString(`<main-critique>`)
		_ = xml.EscapeText(&buffer, []byte(branch.Summary.MainCritique))
		buffer.WriteString(`</main-critique>`)
		buffer.WriteString(`<revised-position>`)
		_ = xml.EscapeText(&buffer, []byte(branch.Summary.RevisedPosition))
		buffer.WriteString(`</revised-position>`)
		if len(branch.Summary.RejectedAssumptions) > 0 {
			buffer.WriteString(`<rejected-assumptions>`)
			for _, ra := range branch.Summary.RejectedAssumptions {
				buffer.WriteString(`<assumption>`)
				_ = xml.EscapeText(&buffer, []byte(ra))
				buffer.WriteString(`</assumption>`)
			}
			buffer.WriteString(`</rejected-assumptions>`)
		}
		if len(branch.Summary.OpenQuestions) > 0 {
			buffer.WriteString(`<open-questions>`)
			for _, oq := range branch.Summary.OpenQuestions {
				buffer.WriteString(`<question>`)
				_ = xml.EscapeText(&buffer, []byte(oq))
				buffer.WriteString(`</question>`)
			}
			buffer.WriteString(`</open-questions>`)
		}
		buffer.WriteString(`</summary>`)

		buffer.WriteString(`</prism>`)
	}
	return buffer.String()
}

// SerializeToolResults serializes cognition.ToolResult list to XML.
func SerializeToolResults(results []cognition.ToolResult) string {
	var buffer bytes.Buffer
	for _, res := range results {
		if res.ErrorCode != "" {
			buffer.WriteString(fmt.Sprintf(`<tool-result id="%s" name="%s" error-code="%s">`, res.ToolCallID, res.Name, res.ErrorCode))
		} else {
			buffer.WriteString(fmt.Sprintf(`<tool-result id="%s" name="%s">`, res.ToolCallID, res.Name))
		}
		_ = xml.EscapeText(&buffer, []byte(res.Content))
		buffer.WriteString(`</tool-result>`)
	}
	return buffer.String()
}
