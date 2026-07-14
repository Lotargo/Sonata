package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/config"
	"github.com/Lotargo/Sonata/internal/protected"
	"github.com/Lotargo/Sonata/internal/provider"
)

type mockModelProvider struct {
	mu       sync.Mutex
	generate func(context.Context, provider.GenerateRequest) (provider.GenerateResult, error)
	calls    []provider.GenerateRequest
}

func (p *mockModelProvider) Generate(ctx context.Context, request provider.GenerateRequest) (provider.GenerateResult, error) {
	p.mu.Lock()
	p.calls = append(p.calls, request)
	p.mu.Unlock()
	return p.generate(ctx, request)
}

func (p *mockModelProvider) Stream(context.Context, provider.GenerateRequest) (<-chan provider.StreamEvent, error) {
	return nil, errors.New("not implemented")
}

func (p *mockModelProvider) ListModels(context.Context) ([]provider.ModelDescriptor, error) {
	return nil, errors.New("not implemented")
}

func testRolePolicies() map[string]config.RoleModelConfig {
	role := func(primary string, fallback ...string) config.RoleModelConfig {
		return config.RoleModelConfig{
			Primary: primary, Fallback: fallback, Timeout: config.Duration(time.Second), MaxOutputTokens: 1024,
		}
	}
	return map[string]config.RoleModelConfig{
		"router":            role("nemotron-3-ultra-free", "mimo-v2.5-free"),
		"raw":               role("deepseek-v4-flash-free", "mimo-v2.5-free"),
		"critical":          role("deepseek-v4-flash-free", "mimo-v2.5-free"),
		"summary":           role("nemotron-3-ultra-free", "mimo-v2.5-free"),
		"synthesis_tooling": role("big-pickle", "deepseek-v4-flash-free", "mimo-v2.5-free"),
		"synthesis_final":   role("big-pickle", "deepseek-v4-flash-free", "mimo-v2.5-free"),
	}
}

func testModelDefinitions() map[string]config.ModelDefinition {
	model := func() config.ModelDefinition {
		return config.ModelDefinition{ProviderRef: "open_code_zen", Protocol: "openai_chat_completions", Enabled: true}
	}
	return map[string]config.ModelDefinition{
		"big-pickle":             model(),
		"mimo-v2.5-free":         model(),
		"nemotron-3-ultra-free":  model(),
		"deepseek-v4-flash-free": model(),
	}
}

func setupTestRunnerAdapter(t *testing.T, upstream *mockModelProvider) *RunnerAdapter {
	t.Helper()

	// Load real protected bundle
	bundle, err := protected.Load(os.DirFS(filepath.Join("..", "..", "protected")), "registry.json")
	if err != nil {
		t.Fatalf("protected.Load() failed: %v", err)
	}

	compiler, err := protected.NewPromptCompiler(bundle)
	if err != nil {
		t.Fatalf("protected.NewPromptCompiler() failed: %v", err)
	}

	router, err := provider.NewModelRouter(upstream, testRolePolicies(), testModelDefinitions(), provider.RouterOptions{})
	if err != nil {
		t.Fatalf("provider.NewModelRouter() failed: %v", err)
	}

	adapter, err := NewRunnerAdapter(router, compiler, bundle)
	if err != nil {
		t.Fatalf("NewRunnerAdapter() failed: %v", err)
	}

	return adapter
}

func TestStripMarkdownJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"{ }", "{ }"},
		{"```json\n{ \"route\": \"direct\" }\n```", "{ \"route\": \"direct\" }"},
		{"```\n{ \"route\": \"direct\" }\n```", "{ \"route\": \"direct\" }"},
		{"```json\n{ \"route\": \"direct\" }", "{ \"route\": \"direct\" }"},
		{"Here is some text before\n```json\n{ \"route\": \"direct\" }\n```\nAnd after", "{ \"route\": \"direct\" }"},
	}

	for _, tt := range tests {
		got := StripMarkdownJSON(tt.input)
		if got != tt.expected {
			t.Errorf("StripMarkdownJSON(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDecodePrismReportStrictness(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		gotContent, gotConfidence, err := DecodePrismReport([]byte(`{"content":"hello","confidence":0.85}`))
		if err != nil {
			t.Fatal(err)
		}
		if gotContent != "hello" || gotConfidence != 0.85 {
			t.Errorf("got (%q, %f), want (hello, 0.85)", gotContent, gotConfidence)
		}
	})

	t.Run("Missing content", func(t *testing.T) {
		_, _, err := DecodePrismReport([]byte(`{"confidence":0.85}`))
		if err == nil || !strings.Contains(err.Error(), "missing content") {
			t.Errorf("expected error containing 'missing content', got %v", err)
		}
	})

	t.Run("Missing confidence", func(t *testing.T) {
		_, _, err := DecodePrismReport([]byte(`{"content":"hello"}`))
		if err == nil || !strings.Contains(err.Error(), "missing confidence") {
			t.Errorf("expected error containing 'missing confidence', got %v", err)
		}
	})

	t.Run("Unknown field", func(t *testing.T) {
		_, _, err := DecodePrismReport([]byte(`{"content":"hello","confidence":0.85,"extra":1}`))
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("expected error containing 'unknown field', got %v", err)
		}
	})

	t.Run("Duplicate key", func(t *testing.T) {
		_, _, err := DecodePrismReport([]byte(`{"content":"hello","content":"world","confidence":0.85}`))
		if err == nil || !strings.Contains(err.Error(), "repeats content") {
			t.Errorf("expected error containing 'repeats content', got %v", err)
		}
	})

	t.Run("Not an object", func(t *testing.T) {
		_, _, err := DecodePrismReport([]byte(`"hello"`))
		if err == nil || !strings.Contains(err.Error(), "must be a JSON object") {
			t.Errorf("expected error containing 'must be a JSON object', got %v", err)
		}
	})

	t.Run("Trailing data", func(t *testing.T) {
		_, _, err := DecodePrismReport([]byte(`{"content":"hello","confidence":0.85} {}`))
		if err == nil || !strings.Contains(err.Error(), "must contain one JSON object") {
			t.Errorf("expected error containing 'must contain one JSON object', got %v", err)
		}
	})
}

func TestDecodeCriticalReportStrictness(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		gotContent, gotWeak, gotUnproven, gotConfidence, err := DecodeCriticalReport([]byte(`{
			"content": "crit",
			"weak_assumptions": ["a", "b"],
			"unproven_conclusions": ["c"],
			"confidence": 0.9
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if gotContent != "crit" || !reflect.DeepEqual(gotWeak, []string{"a", "b"}) || !reflect.DeepEqual(gotUnproven, []string{"c"}) || gotConfidence != 0.9 {
			t.Errorf("got mismatched results: content=%q weak=%v unproven=%v confidence=%f", gotContent, gotWeak, gotUnproven, gotConfidence)
		}
	})

	t.Run("Missing lists defaults to empty array", func(t *testing.T) {
		gotContent, gotWeak, gotUnproven, gotConfidence, err := DecodeCriticalReport([]byte(`{
			"content": "crit",
			"confidence": 0.9
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if gotContent != "crit" || gotWeak == nil || len(gotWeak) != 0 || gotUnproven == nil || len(gotUnproven) != 0 || gotConfidence != 0.9 {
			t.Errorf("got mismatched defaults: content=%q weak=%v unproven=%v confidence=%f", gotContent, gotWeak, gotUnproven, gotConfidence)
		}
	})
}

func TestDecodePrismSummaryStrictness(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		gotInit, gotCrit, gotRev, gotRejected, gotQuestions, gotConf, err := DecodePrismSummary([]byte(`{
			"initial_position": "init",
			"main_critique": "critique",
			"revised_position": "revised",
			"rejected_assumptions": ["a"],
			"open_questions": ["q"],
			"confidence": 0.8
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if gotInit != "init" || gotCrit != "critique" || gotRev != "revised" || !reflect.DeepEqual(gotRejected, []string{"a"}) || !reflect.DeepEqual(gotQuestions, []string{"q"}) || gotConf != 0.8 {
			t.Error("mismatched summary fields decoded")
		}
	})
}

func TestDecodeSynthesisToolingOutputStrictness(t *testing.T) {
	t.Run("Valid with JSON tool calls", func(t *testing.T) {
		gotDec, gotCalls, err := DecodeSynthesisToolingOutput([]byte(`{
			"preliminary_decision": "decide",
			"tool_calls": [
				{
					"id": "1",
					"name": "web.search.langsearch",
					"arguments": {"q": "testing"}
				}
			]
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if gotDec != "decide" || len(gotCalls) != 1 || gotCalls[0].ID != "1" || gotCalls[0].Name != "web.search.langsearch" || gotCalls[0].Arguments != `{"q":"testing"}` {
			t.Errorf("mismatched decoded tooling output: dec=%q calls=%+v", gotDec, gotCalls)
		}
	})

	t.Run("Valid with string arguments", func(t *testing.T) {
		gotDec, gotCalls, err := DecodeSynthesisToolingOutput([]byte(`{
			"preliminary_decision": "decide",
			"tool_calls": [
				{
					"id": "1",
					"name": "web.search.langsearch",
					"arguments": "{\"q\": \"testing\"}"
				}
			]
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if gotDec != "decide" || len(gotCalls) != 1 || gotCalls[0].ID != "1" || gotCalls[0].Arguments != `{"q": "testing"}` {
			t.Errorf("mismatched decoded tooling output: dec=%q calls=%+v", gotDec, gotCalls)
		}
	})
}

func TestRunnerAdapterExecution(t *testing.T) {
	t.Run("RunRouter Direct", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"route":"direct"}`,
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)
		res, err := adapter.RunRouter(context.Background(), cognition.RouterInput{
			UserInput: "hello",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Output.Route != cognition.RouteDirect {
			t.Errorf("route = %q, want direct", res.Output.Route)
		}
		if res.Metadata.Role != cognition.RoleRouter || res.Metadata.Status != cognition.RoleStatusSucceeded {
			t.Errorf("metadata = %+v", res.Metadata)
		}
	})

	t.Run("RunRaw with default manifest", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"content":"raw response text","confidence":0.95}`,
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)
		res, err := adapter.RunRaw(context.Background(), cognition.RawInput{
			Prism:       cognition.PrismEfficiency,
			UserInput:   "do something",
			Instruction: cognition.ArtifactRef{ID: "prism.efficiency.raw", Version: 1, Hash: "3e37b6c33862e0a770fadc530d93f183d2169ca4139ddea0e6d1897999e88799"},
			Manifest:    cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "manifest.prism.efficiency.raw.default", Version: 1, Hash: "597b0b3ccea711ab1cb3e86aab9f6fbac9364319852ec9a5b63e2796d6d65c9c"}, Source: "system_default"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Content != "raw response text" || res.Confidence != 0.95 {
			t.Errorf("report = %+v", res)
		}
	})

	t.Run("RunRaw with user manifest from context", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"content":"raw response text","confidence":0.95}`,
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)

		// Place user manifest in context
		ctx := ContextWithUserManifests(context.Background(), map[string]string{
			"user-manifest-id-123": "Tone: extreme efficiency guidelines",
		})

		res, err := adapter.RunRaw(ctx, cognition.RawInput{
			Prism:       cognition.PrismEfficiency,
			UserInput:   "do something",
			Instruction: cognition.ArtifactRef{ID: "prism.efficiency.raw", Version: 1, Hash: "3e37b6c33862e0a770fadc530d93f183d2169ca4139ddea0e6d1897999e88799"},
			Manifest:    cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "user-manifest-id-123", Version: 1, Hash: "5e478546b3f74f7627cb224976a4cd5592ec69ca4139ddea0e6d1897999e8879b"}, Source: "user_global"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Content != "raw response text" || res.Confidence != 0.95 {
			t.Errorf("report = %+v", res)
		}
	})

	t.Run("RunCritical", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"content":"critique","weak_assumptions":["x"],"unproven_conclusions":["y"],"confidence":0.90}`,
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)
		res, err := adapter.RunCritical(context.Background(), cognition.CriticalInput{
			Prism:       cognition.PrismEfficiency,
			UserInput:   "do something",
			Raw:         cognition.PrismReport{Content: "raw report content", Confidence: 0.95},
			Instruction: cognition.ArtifactRef{ID: "prism.efficiency.critical", Version: 1, Hash: "243f22cc4215870b4b80b1979e942f5f68b717118ad8f716c7dcd829a57f31b6"},
			Manifest:    cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "manifest.prism.efficiency.critical.default", Version: 1, Hash: "6b4987696584a82a9cbe7ff18c769d950fc679e9723de1a88bbf5d72447912f9"}, Source: "system_default"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Content != "critique" || res.Confidence != 0.90 || !reflect.DeepEqual(res.WeakAssumptions, []string{"x"}) {
			t.Errorf("report = %+v", res)
		}
	})

	t.Run("RunSummary", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"initial_position":"init","main_critique":"critique","revised_position":"revised","rejected_assumptions":["x"],"open_questions":["y"],"confidence":0.80}`,
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)
		res, err := adapter.RunSummary(context.Background(), cognition.SummaryInput{
			Prism:       cognition.PrismEfficiency,
			Raw:         cognition.PrismReport{Content: "raw report content", Confidence: 0.95},
			Critical:    cognition.CriticalReport{Content: "critical content", Confidence: 0.90},
			Instruction: cognition.ArtifactRef{ID: "prism.efficiency.summary", Version: 1, Hash: "31614c908b42b34e0b7abd95fd68bc508b1ba89a8c94650fb564ed391fd9c18f"},
			Manifest:    cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "manifest.prism.efficiency.summary.default", Version: 1, Hash: "f6e6a9e391f1ac7845b1375d10b0b915899ff2d07ba9fefe07c65d30b4b672f4"}, Source: "system_default"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.InitialPosition != "init" || res.Confidence != 0.80 || !reflect.DeepEqual(res.OpenQuestions, []string{"y"}) {
			t.Errorf("report = %+v", res)
		}
	})

	t.Run("RunSynthesisTooling", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"preliminary_decision":"prelim","tool_calls":[{"id":"call-1","name":"web.search.langsearch","arguments":{"q":"test"}}]}`,
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)
		res, err := adapter.RunSynthesisTooling(context.Background(), cognition.SynthesisToolingInput{
			UserInput:   "search search",
			Dialogue:    cognition.InternalDialogue{Branches: make(map[cognition.Prism]cognition.PrismDialogue)},
			Instruction: cognition.ArtifactRef{ID: "synthesis.tooling", Version: 1, Hash: "9cf8c79e46bb867892ee13d61ca3ccebb7b322dad5ee44ed4d01d7a03fda9a09"},
			Manifest:    cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "manifest.synthesis.tooling.default", Version: 1, Hash: "a0f5c431e83b9f4e39201b4570009c841cd2b63f29e0389b6015ec46e07ed5de"}, Source: "system_default"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.PreliminaryDecision != "prelim" || len(res.ToolCalls) != 1 || res.ToolCalls[0].ID != "call-1" || res.ToolCalls[0].Name != "web.search.langsearch" {
			t.Errorf("tooling output = %+v", res)
		}
	})

	t.Run("RunSynthesisFinal", func(t *testing.T) {
		providerMock := &mockModelProvider{
			generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: "Hello world answer.",
				}, nil
			},
		}
		adapter := setupTestRunnerAdapter(t, providerMock)
		res, err := adapter.RunSynthesisFinal(context.Background(), cognition.SynthesisFinalInput{
			Route:               cognition.RouteFull,
			UserInput:           "give final answer",
			Dialogue:            cognition.InternalDialogue{Branches: make(map[cognition.Prism]cognition.PrismDialogue)},
			PreliminaryDecision: "already decided",
			ToolResults:         []cognition.ToolResult{{ToolCallID: "call-1", Name: "web.search.langsearch", Content: "web results"}},
			Instruction:         cognition.ArtifactRef{ID: "synthesis.final", Version: 1, Hash: "1e959d8636bc0c983cbccc2ab2bce9f06b41e1b21c63046414c919fb2fb57ed0"},
			Manifest:            cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "manifest.synthesis.final.default", Version: 1, Hash: "b2ab7b2c7cd705815256407e520ed74c07730d41bdb6d7ddbb5754e21efc5e28"}, Source: "system_default"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Content != "Hello world answer." {
			t.Errorf("content = %q, want Hello world answer.", res.Content)
		}
	})
}
