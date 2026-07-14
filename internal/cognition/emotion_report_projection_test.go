package cognition

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

func validCanonicalEmotionReport() EmotionReport {
	return EmotionReport{
		Text:         "status=HEALTHY; dominant=trust,joy; tone=warm_attentive",
		StateVersion: 7,
	}
}

func TestEmotionReportUsesOneValidatedContractVersion(t *testing.T) {
	report := validCanonicalEmotionReport()
	if report.ContractVersion() != EmotionReportContractVersion {
		t.Fatalf("contract version = %q, want %q", report.ContractVersion(), EmotionReportContractVersion)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := []EmotionReport{
		{},
		{Text: "   ", StateVersion: 1},
		{Text: "valid", StateVersion: -1},
	}
	for _, value := range invalid {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid report was accepted: %#v", value)
		}
	}
}

func TestEmotionReportProjectionBoundariesAreStructural(t *testing.T) {
	emotionType := reflect.TypeOf(EmotionReport{})
	allowed := []struct {
		name  string
		value any
	}{
		{name: "RawInput", value: RawInput{}},
		{name: "CriticalInput", value: CriticalInput{}},
		{name: "SynthesisToolingInput", value: SynthesisToolingInput{}},
		{name: "SynthesisFinalInput", value: SynthesisFinalInput{}},
	}
	for _, target := range allowed {
		field, exists := reflect.TypeOf(target.value).FieldByName("Emotion")
		if !exists || field.Type != emotionType {
			t.Fatalf("%s must expose exactly one EmotionReport field", target.name)
		}
	}

	forbidden := []struct {
		name  string
		value any
	}{
		{name: "RouterInput", value: RouterInput{}},
		{name: "SummaryInput", value: SummaryInput{}},
	}
	for _, target := range forbidden {
		typeOf := reflect.TypeOf(target.value)
		for _, fieldName := range []string{"Emotion", "EmotionReport"} {
			if _, exists := typeOf.FieldByName(fieldName); exists {
				t.Fatalf("%s exposes forbidden field %s", target.name, fieldName)
			}
		}
	}
}

func TestFullPipelineProjectsTheSameEmotionReportToEveryAllowedRole(t *testing.T) {
	report := validCanonicalEmotionReport()
	var mu sync.Mutex
	seen := make(map[RuntimeRole]int)
	var mismatched []RuntimeRole
	record := func(role RuntimeRole, value EmotionReport) {
		mu.Lock()
		defer mu.Unlock()
		seen[role]++
		if value != report {
			mismatched = append(mismatched, role)
		}
	}

	router := routerRunnerFunc(func(context.Context, RouterInput) (RouterRunResult, error) {
		return RouterRunResult{
			Output:   RouterOutput{Route: RouteFull},
			Metadata: successfulMetadata(RoleRouter, "router-model"),
		}, nil
	})
	raw := rawRunnerFunc(func(_ context.Context, input RawInput) (PrismReport, error) {
		role := roleForPrismPhase(input.Prism, PhaseRaw)
		record(role, input.Emotion)
		return validRawReport(input, role), nil
	})
	critical := criticalRunnerFunc(func(_ context.Context, input CriticalInput) (CriticalReport, error) {
		role := roleForPrismPhase(input.Prism, PhaseCritical)
		record(role, input.Emotion)
		return validCriticalReport(input, role), nil
	})
	summary := summaryRunnerFunc(func(_ context.Context, input SummaryInput) (PrismSummary, error) {
		return validSummary(input, roleForPrismPhase(input.Prism, PhaseSummary)), nil
	})
	tooling := synthesisToolingRunnerFunc(func(_ context.Context, input SynthesisToolingInput) (SynthesisToolingOutput, error) {
		record(RoleSynthesisTooling, input.Emotion)
		return SynthesisToolingOutput{
			PreliminaryDecision: "answer without tools",
			Metadata: metadataForArtifacts(
				RoleSynthesisTooling,
				"tooling-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})
	final := synthesisFinalRunnerFunc(func(_ context.Context, input SynthesisFinalInput) (SynthesisFinalOutput, error) {
		record(RoleSynthesisFinal, input.Emotion)
		return SynthesisFinalOutput{
			Content: "final answer",
			Metadata: metadataForArtifacts(
				RoleSynthesisFinal,
				"final-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})

	pipeline, err := NewPipeline(router, raw, critical, summary, tooling, final, nil, FullPipelineOptions{
		PhaseTimeout:         time.Second,
		MaxConcurrentPrisms:  5,
		MinimumHealthyPrisms: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := pipeline.Run(context.Background(), PipelineInput{
		UserInput: "complex question",
		Emotion:   report,
		Artifacts: testAllRoleArtifacts(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Route != RouteFull || result.Full == nil {
		t.Fatalf("result = %#v", result)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(mismatched) != 0 {
		t.Fatalf("roles received a different emotion report: %v", mismatched)
	}
	var got []string
	for role, count := range seen {
		if count != 1 {
			t.Fatalf("role %s received report %d times", role, count)
		}
		got = append(got, string(role))
	}
	sort.Strings(got)
	var want []string
	for _, spec := range RuntimeRoleSpecs() {
		if spec.Phase == PhaseRaw || spec.Phase == PhaseCritical || spec.Phase == PhaseSynthesisTooling || spec.Phase == PhaseSynthesisFinal {
			want = append(want, string(spec.Role))
		}
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("emotion report recipients = %v, want %v", got, want)
	}
}
