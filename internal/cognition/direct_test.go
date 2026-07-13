package cognition

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type routerRunnerFunc func(context.Context, RouterInput) (RouterRunResult, error)

func (function routerRunnerFunc) RunRouter(ctx context.Context, input RouterInput) (RouterRunResult, error) {
	return function(ctx, input)
}

type synthesisFinalRunnerFunc func(context.Context, SynthesisFinalInput) (SynthesisFinalOutput, error)

func (function synthesisFinalRunnerFunc) RunSynthesisFinal(ctx context.Context, input SynthesisFinalInput) (SynthesisFinalOutput, error) {
	return function(ctx, input)
}

func TestDirectPipelineRunsOnlyRouterAndSynthesisFinal(t *testing.T) {
	var calls []RuntimeRole
	var receivedFinal SynthesisFinalInput
	router := routerRunnerFunc(func(_ context.Context, input RouterInput) (RouterRunResult, error) {
		calls = append(calls, RoleRouter)
		if input.UserInput != "hello" || len(input.History) != 1 {
			t.Fatalf("router input = %#v", input)
		}
		input.History[0].Content = "mutated"
		return RouterRunResult{
			Output:   RouterOutput{Route: RouteDirect},
			Metadata: successfulMetadata(RoleRouter, "router-model"),
		}, nil
	})
	final := synthesisFinalRunnerFunc(func(_ context.Context, input SynthesisFinalInput) (SynthesisFinalOutput, error) {
		calls = append(calls, RoleSynthesisFinal)
		receivedFinal = input
		return SynthesisFinalOutput{
			Content: "Hi.",
			Metadata: metadataForArtifacts(
				RoleSynthesisFinal,
				"final-model",
				RoleArtifacts{Instruction: input.Instruction, Manifest: input.Manifest},
			),
		}, nil
	})
	pipeline, err := NewDirectPipeline(router, final)
	if err != nil {
		t.Fatal(err)
	}
	history := []ConversationMessage{{Role: "user", Content: "previous"}}
	result, err := pipeline.Run(context.Background(), DirectPipelineInput{
		UserInput:        "hello",
		History:          history,
		FinalInstruction: testArtifactRef("synthesis.final"),
		FinalManifest:    testManifestRef("manifest.synthesis.final.default"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []RuntimeRole{RoleRouter, RoleSynthesisFinal}) {
		t.Fatalf("calls = %#v", calls)
	}
	if result.Route != RouteDirect || result.Final.Content != "Hi." {
		t.Fatalf("result = %#v", result)
	}
	if history[0].Content != "previous" || receivedFinal.History[0].Content != "previous" {
		t.Fatalf("history aliasing detected: source=%#v final=%#v", history, receivedFinal.History)
	}
	if receivedFinal.Route != RouteDirect {
		t.Fatalf("final route = %q", receivedFinal.Route)
	}
	if receivedFinal.Context.Text != "" || len(receivedFinal.Context.CitationIDs) != 0 || receivedFinal.Emotion != (EmotionReport{}) {
		t.Fatalf("direct route received full-route context: %#v", receivedFinal)
	}
	if len(receivedFinal.Dialogue.Branches) != 0 || receivedFinal.PreliminaryDecision != "" || len(receivedFinal.ToolResults) != 0 {
		t.Fatalf("direct route received internal dialogue or tools: %#v", receivedFinal)
	}
}

func TestDirectPipelineHandsFullRouteBackWithoutCallingFinal(t *testing.T) {
	finalCalled := false
	pipeline, err := NewDirectPipeline(
		routerRunnerFunc(func(context.Context, RouterInput) (RouterRunResult, error) {
			return RouterRunResult{
				Output:   RouterOutput{Route: RouteFull},
				Metadata: successfulMetadata(RoleRouter, "router-model"),
			}, nil
		}),
		synthesisFinalRunnerFunc(func(context.Context, SynthesisFinalInput) (SynthesisFinalOutput, error) {
			finalCalled = true
			return SynthesisFinalOutput{}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := pipeline.Run(context.Background(), validDirectPipelineInput())
	if !errors.Is(err, ErrFullRouteRequired) {
		t.Fatalf("error = %v", err)
	}
	if result.Route != RouteFull || finalCalled {
		t.Fatalf("result=%#v finalCalled=%t", result, finalCalled)
	}
}

func TestDirectPipelinePropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	routerCalled := false
	pipeline, err := NewDirectPipeline(
		routerRunnerFunc(func(context.Context, RouterInput) (RouterRunResult, error) {
			routerCalled = true
			return RouterRunResult{}, nil
		}),
		synthesisFinalRunnerFunc(func(context.Context, SynthesisFinalInput) (SynthesisFinalOutput, error) {
			return SynthesisFinalOutput{}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(ctx, validDirectPipelineInput()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if routerCalled {
		t.Fatal("router was called after cancellation")
	}
}

func TestDirectPipelineRejectsInvalidRoleMetadata(t *testing.T) {
	pipeline, err := NewDirectPipeline(
		routerRunnerFunc(func(context.Context, RouterInput) (RouterRunResult, error) {
			return RouterRunResult{
				Output:   RouterOutput{Route: RouteDirect},
				Metadata: successfulMetadata(RoleEfficiencyRaw, "wrong-role-model"),
			}, nil
		}),
		synthesisFinalRunnerFunc(func(context.Context, SynthesisFinalInput) (SynthesisFinalOutput, error) {
			return SynthesisFinalOutput{}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(context.Background(), validDirectPipelineInput()); err == nil {
		t.Fatal("invalid router role metadata was accepted")
	}
}

func validDirectPipelineInput() DirectPipelineInput {
	return DirectPipelineInput{
		UserInput:        "hello",
		FinalInstruction: testArtifactRef("synthesis.final"),
		FinalManifest:    testManifestRef("manifest.synthesis.final.default"),
	}
}

func successfulMetadata(role RuntimeRole, model string) RoleMetadata {
	return RoleMetadata{
		Role:        role,
		Status:      RoleStatusSucceeded,
		ModelID:     model,
		Latency:     time.Millisecond,
		Instruction: testArtifactRef(string(role)),
		Manifest:    testManifestRef("manifest." + string(role)),
	}
}

func metadataForArtifacts(role RuntimeRole, model string, artifacts RoleArtifacts) RoleMetadata {
	metadata := successfulMetadata(role, model)
	metadata.Instruction = artifacts.Instruction
	metadata.Manifest = artifacts.Manifest
	return metadata
}

func testArtifactRef(id string) ArtifactRef {
	return ArtifactRef{ID: id, Version: 1, Hash: "hash"}
}

func testManifestRef(id string) ManifestRef {
	return ManifestRef{ArtifactRef: testArtifactRef(id), Source: "system_default"}
}
