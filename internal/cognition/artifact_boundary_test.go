package cognition

import (
	"context"
	"testing"
)

func TestFullPipelineRejectsInstructionArtifactFromAnotherRole(t *testing.T) {
	input := validFullPipelineInput()
	artifacts := input.Artifacts[RoleEfficiencyRaw]
	artifacts.Instruction = testArtifactRef("prism.ethics.critical")
	input.Artifacts[RoleEfficiencyRaw] = artifacts
	pipeline := newTestFullPipeline(t, nil, nil, FullPipelineOptions{})
	if _, err := pipeline.RunAfterRouter(context.Background(), input); err == nil {
		t.Fatal("cross-role protected instruction was accepted")
	}
}
