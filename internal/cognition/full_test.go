package cognition

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

type rawRunnerFunc func(context.Context, RawInput) (PrismReport, error)

func (function rawRunnerFunc) RunRaw(ctx context.Context, input RawInput) (PrismReport, error) {
	return function(ctx, input)
}

type criticalRunnerFunc func(context.Context, CriticalInput) (CriticalReport, error)

func (function criticalRunnerFunc) RunCritical(ctx context.Context, input CriticalInput) (CriticalReport, error) {
	return function(ctx, input)
}

type summaryRunnerFunc func(context.Context, SummaryInput) (PrismSummary, error)

func (function summaryRunnerFunc) RunSummary(ctx context.Context, input SummaryInput) (PrismSummary, error) {
	return function(ctx, input)
}

type synthesisToolingRunnerFunc func(context.Context, SynthesisToolingInput) (SynthesisToolingOutput, error)

func (function synthesisToolingRunnerFunc) RunSynthesisTooling(ctx context.Context, input SynthesisToolingInput) (SynthesisToolingOutput, error) {
	return function(ctx, input)
}

type toolExecutorFunc func(context.Context, []ToolCall) ([]ToolResult, error)

func (function toolExecutorFunc) ExecuteTools(ctx context.Context, calls []ToolCall) ([]ToolResult, error) {
	return function(ctx, calls)
}

type roleRecorder struct {
	mu    sync.Mutex
	roles []RuntimeRole
}

func (recorder *roleRecorder) add(role RuntimeRole) {
	recorder.mu.Lock()
	recorder.roles = append(recorder.roles, role)
	recorder.mu.Unlock()
}

func (recorder *roleRecorder) snapshot() []RuntimeRole {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]RuntimeRole(nil), recorder.roles...)
}

func TestPipelineFullRouteExecutesExactly18IsolatedRoleCalls(t *testing.T) {
	recorder := &roleRecorder{}
	artifacts := testAllRoleArtifacts()
	router := routerRunnerFunc(func(_ context.Context, input RouterInput) (RouterRunResult, error) {
		recorder.add(RoleRouter)
		if input.UserInput != "complex question" {
			t.Fatalf("router input = %#v", input)
		}
		return RouterRunResult{
			Output:   RouterOutput{Route: RouteFull},
			Metadata: successfulMetadata(RoleRouter, "router-model"),
		}, nil
	})
	raw := rawRunnerFunc(func(_ context.Context, input RawInput) (PrismReport, error) {
		role := roleForPrismPhase(input.Prism, PhaseRaw)
		recorder.add(role)
		return validRawReport(input, role), nil
	})
	critical := criticalRunnerFunc(func(_ context.Context, input CriticalInput) (CriticalReport, error) {
		role := roleForPrismPhase(input.Prism, PhaseCritical)
		recorder.add(role)
		if input.Raw.Prism != input.Prism || input.Raw.Content != string(input.Prism)+" raw" {
			t.Fatalf("critical %s received wrong raw report: %#v", input.Prism, input.Raw)
		}
		return validCriticalReport(input, role), nil
	})
	summary := summaryRunnerFunc(func(_ context.Context, input SummaryInput) (PrismSummary, error) {
		role := roleForPrismPhase(input.Prism, PhaseSummary)
		recorder.add(role)
		if input.Raw.Prism != input.Prism || input.Critical.Prism != input.Prism {
			t.Fatalf("summary %s crossed prism boundary: %#v", input.Prism, input)
		}
		return validSummary(input, role), nil
	})
	tooling := synthesisToolingRunnerFunc(func(_ context.Context, input SynthesisToolingInput) (SynthesisToolingOutput, error) {
		recorder.add(RoleSynthesisTooling)
		if err := input.Dialogue.ValidateFull(); err != nil {
			t.Fatal(err)
		}
		return SynthesisToolingOutput{
			PreliminaryDecision: "search before final answer",
			ToolCalls:           []ToolCall{{ID: "tool-1", Name: "web.search.langsearch", Arguments: `{"q":"test"}`}},
			Metadata: metadataForArtifacts(
				RoleSynthesisTooling,
				"tooling-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})
	executor := toolExecutorFunc(func(_ context.Context, calls []ToolCall) ([]ToolResult, error) {
		if len(calls) != 1 || calls[0].ID != "tool-1" {
			t.Fatalf("tool calls = %#v", calls)
		}
		return []ToolResult{{ToolCallID: "tool-1", Name: calls[0].Name, Content: "normalized result"}}, nil
	})
	final := synthesisFinalRunnerFunc(func(_ context.Context, input SynthesisFinalInput) (SynthesisFinalOutput, error) {
		recorder.add(RoleSynthesisFinal)
		if input.Route != RouteFull || input.PreliminaryDecision != "search before final answer" {
			t.Fatalf("final input = %#v", input)
		}
		if err := input.Dialogue.ValidateFull(); err != nil {
			t.Fatal(err)
		}
		if len(input.ToolResults) != 1 || input.ToolResults[0].Content != "normalized result" {
			t.Fatalf("tool results = %#v", input.ToolResults)
		}
		return SynthesisFinalOutput{
			Content: "final answer",
			Metadata: metadataForArtifacts(
				RoleSynthesisFinal,
				"final-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})
	pipeline, err := NewPipeline(router, raw, critical, summary, tooling, final, executor, FullPipelineOptions{
		PhaseTimeout:         time.Second,
		MaxConcurrentPrisms:  5,
		MinimumHealthyPrisms: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := pipeline.Run(context.Background(), PipelineInput{
		UserInput: "complex question",
		History:   []ConversationMessage{{Role: "user", Content: "earlier"}},
		Context:   ContextPack{Text: "context", CitationIDs: []string{"citation-1"}},
		Emotion:   EmotionReport{Text: "stable", StateVersion: 2},
		Artifacts: artifacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Route != RouteFull || result.Status != RoleStatusSucceeded || result.Full == nil {
		t.Fatalf("result = %#v", result)
	}
	if result.Full.Final.Content != "final answer" || len(result.Full.Dialogue.Branches) != 5 {
		t.Fatalf("full result = %#v", result.Full)
	}
	assertExactlyOneOfEachRuntimeRole(t, recorder.snapshot())
}

func TestFullPipelineStartsFiveRawPrismsConcurrently(t *testing.T) {
	started := make(chan Prism, 5)
	release := make(chan struct{})
	raw := rawRunnerFunc(func(ctx context.Context, input RawInput) (PrismReport, error) {
		started <- input.Prism
		select {
		case <-release:
			return validRawReport(input, roleForPrismPhase(input.Prism, PhaseRaw)), nil
		case <-ctx.Done():
			return PrismReport{}, ctx.Err()
		}
	})
	pipeline := newTestFullPipeline(t, raw, nil, FullPipelineOptions{
		PhaseTimeout:         time.Second,
		MaxConcurrentPrisms:  5,
		MinimumHealthyPrisms: 4,
	})
	done := make(chan error, 1)
	go func() {
		_, err := pipeline.RunAfterRouter(context.Background(), validFullPipelineInput())
		done <- err
	}()
	seen := make(map[Prism]struct{})
	for range 5 {
		select {
		case prism := <-started:
			seen[prism] = struct{}{}
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("only %d raw prisms started before release", len(seen))
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(seen) != 5 {
		t.Fatalf("concurrent raw prisms = %#v", seen)
	}
}

func TestFullPipelineDegradesWhenOnePrismFails(t *testing.T) {
	raw := rawRunnerFunc(func(_ context.Context, input RawInput) (PrismReport, error) {
		if input.Prism == PrismEthics {
			return PrismReport{}, errors.New("mock ethics failure")
		}
		return validRawReport(input, roleForPrismPhase(input.Prism, PhaseRaw)), nil
	})
	pipeline := newTestFullPipeline(t, raw, nil, FullPipelineOptions{
		PhaseTimeout:         time.Second,
		MaxConcurrentPrisms:  5,
		MinimumHealthyPrisms: 4,
	})
	result, err := pipeline.RunAfterRouter(context.Background(), validFullPipelineInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RoleStatusDegraded || len(result.Dialogue.Branches) != 4 {
		t.Fatalf("result = %#v", result)
	}
	failure, exists := result.Failures[PrismEthics]
	if !exists || failure.Phase != PhaseRaw {
		t.Fatalf("ethics failure = %#v", failure)
	}
	if _, exists := result.Dialogue.Branches[PrismEthics]; exists {
		t.Fatal("failed ethics branch reached synthesis")
	}
}

func TestFullPipelineAppliesPerPhaseTimeoutAndContinuesDegraded(t *testing.T) {
	raw := rawRunnerFunc(func(ctx context.Context, input RawInput) (PrismReport, error) {
		if input.Prism == PrismEthics {
			<-ctx.Done()
			return PrismReport{}, ctx.Err()
		}
		return validRawReport(input, roleForPrismPhase(input.Prism, PhaseRaw)), nil
	})
	pipeline := newTestFullPipeline(t, raw, nil, FullPipelineOptions{
		PhaseTimeout:         25 * time.Millisecond,
		MaxConcurrentPrisms:  5,
		MinimumHealthyPrisms: 4,
	})
	started := time.Now()
	result, err := pipeline.RunAfterRouter(context.Background(), validFullPipelineInput())
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("phase timeout was not enforced: %s", elapsed)
	}
	failure := result.Failures[PrismEthics]
	if !errors.Is(failure.Err, context.DeadlineExceeded) || result.Status != RoleStatusDegraded {
		t.Fatalf("timeout result=%#v failure=%#v", result, failure)
	}
}

func TestFullPipelineStopsWhenTooFewPrismsRemain(t *testing.T) {
	toolingCalled := false
	raw := rawRunnerFunc(func(_ context.Context, input RawInput) (PrismReport, error) {
		if input.Prism == PrismEthics || input.Prism == PrismPhilosophy {
			return PrismReport{}, errors.New("mock branch failure")
		}
		return validRawReport(input, roleForPrismPhase(input.Prism, PhaseRaw)), nil
	})
	pipeline := newTestFullPipeline(t, raw, func() { toolingCalled = true }, FullPipelineOptions{
		PhaseTimeout:         time.Second,
		MaxConcurrentPrisms:  5,
		MinimumHealthyPrisms: 4,
	})
	result, err := pipeline.RunAfterRouter(context.Background(), validFullPipelineInput())
	if !errors.Is(err, ErrInsufficientPrisms) {
		t.Fatalf("error = %v", err)
	}
	if result.Status != RoleStatusFailed || toolingCalled {
		t.Fatalf("result=%#v toolingCalled=%t", result, toolingCalled)
	}
}

func newTestFullPipeline(t *testing.T, raw RawRunner, toolingHook func(), options FullPipelineOptions) *FullPipeline {
	t.Helper()
	if raw == nil {
		raw = rawRunnerFunc(func(_ context.Context, input RawInput) (PrismReport, error) {
			return validRawReport(input, roleForPrismPhase(input.Prism, PhaseRaw)), nil
		})
	}
	critical := criticalRunnerFunc(func(_ context.Context, input CriticalInput) (CriticalReport, error) {
		return validCriticalReport(input, roleForPrismPhase(input.Prism, PhaseCritical)), nil
	})
	summary := summaryRunnerFunc(func(_ context.Context, input SummaryInput) (PrismSummary, error) {
		return validSummary(input, roleForPrismPhase(input.Prism, PhaseSummary)), nil
	})
	tooling := synthesisToolingRunnerFunc(func(_ context.Context, input SynthesisToolingInput) (SynthesisToolingOutput, error) {
		if toolingHook != nil {
			toolingHook()
		}
		return SynthesisToolingOutput{
			PreliminaryDecision: "preliminary",
			Metadata: metadataForArtifacts(
				RoleSynthesisTooling,
				"tooling-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})
	final := synthesisFinalRunnerFunc(func(_ context.Context, input SynthesisFinalInput) (SynthesisFinalOutput, error) {
		return SynthesisFinalOutput{
			Content: "final",
			Metadata: metadataForArtifacts(
				RoleSynthesisFinal,
				"final-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})
	pipeline, err := NewFullPipeline(raw, critical, summary, tooling, final, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

func validFullPipelineInput() FullPipelineInput {
	return FullPipelineInput{
		UserInput: "question",
		History:   []ConversationMessage{{Role: "user", Content: "history"}},
		Context:   ContextPack{Text: "context", CitationIDs: []string{"c1"}},
		Emotion:   EmotionReport{Text: "stable", StateVersion: 1},
		Router:    successfulMetadata(RoleRouter, "router-model"),
		Artifacts: testAllRoleArtifacts(),
	}
}

func testAllRoleArtifacts() map[RuntimeRole]RoleArtifacts {
	artifacts := make(map[RuntimeRole]RoleArtifacts)
	for _, spec := range RuntimeRoleSpecs() {
		if spec.Role == RoleRouter {
			continue
		}
		artifacts[spec.Role] = RoleArtifacts{
			Instruction: testArtifactRef(spec.InstructionID),
			Manifest:    testManifestRef("manifest." + spec.InstructionID + ".default"),
		}
	}
	return artifacts
}

func validRawReport(input RawInput, role RuntimeRole) PrismReport {
	return PrismReport{
		Prism:      input.Prism,
		Content:    string(input.Prism) + " raw",
		Confidence: 0.8,
		Metadata: metadataForArtifacts(
			role,
			string(role)+"-model",
			RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
		),
	}
}

func validCriticalReport(input CriticalInput, role RuntimeRole) CriticalReport {
	return CriticalReport{
		Prism:           input.Prism,
		Content:         string(input.Prism) + " critical",
		WeakAssumptions: []string{"assumption"},
		Confidence:      0.7,
		Metadata: metadataForArtifacts(
			role,
			string(role)+"-model",
			RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
		),
	}
}

func validSummary(input SummaryInput, role RuntimeRole) PrismSummary {
	return PrismSummary{
		Prism:           input.Prism,
		InitialPosition: input.Raw.Content,
		MainCritique:    input.Critical.Content,
		RevisedPosition: string(input.Prism) + " revised",
		OpenQuestions:   []string{"question"},
		Confidence:      0.75,
		Metadata: metadataForArtifacts(
			role,
			string(role)+"-model",
			RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
		),
	}
}

func assertExactlyOneOfEachRuntimeRole(t *testing.T, roles []RuntimeRole) {
	t.Helper()
	if len(roles) != 18 {
		t.Fatalf("runtime calls = %d, want 18: %#v", len(roles), roles)
	}
	got := make([]string, len(roles))
	for index, role := range roles {
		got[index] = string(role)
	}
	sort.Strings(got)
	want := make([]string, 0, 18)
	for _, spec := range RuntimeRoleSpecs() {
		want = append(want, string(spec.Role))
	}
	sort.Strings(want)
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("runtime roles = %v, want %v", got, want)
		}
	}
}

func ExampleRuntimeRoleSpecs() {
	fmt.Println(len(RuntimeRoleSpecs()))
	// Output: 18
}
