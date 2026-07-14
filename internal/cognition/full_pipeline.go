package cognition

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (pipeline *FullPipeline) RunAfterRouter(ctx context.Context, input FullPipelineInput) (FullPipelineResult, error) {
	result := FullPipelineResult{
		Status:   RoleStatusSucceeded,
		Router:   input.Router,
		Failures: make(map[Prism]BranchFailure),
	}
	if pipeline == nil || pipeline.raw == nil || pipeline.critical == nil || pipeline.summary == nil || pipeline.tooling == nil || pipeline.final == nil {
		return result, errors.New("full pipeline is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if strings.TrimSpace(input.UserInput) == "" {
		return result, errors.New("full pipeline user input is required")
	}
	if err := input.Emotion.Validate(); err != nil {
		return result, fmt.Errorf("validate emotion report: %w", err)
	}
	if err := validateRoleMetadata(input.Router, RoleRouter); err != nil {
		return result, fmt.Errorf("validate router metadata: %w", err)
	}
	if input.Router.Status == RoleStatusDegraded {
		result.Status = RoleStatusDegraded
	}
	for _, spec := range RuntimeRoleSpecs() {
		if spec.Role == RoleRouter {
			continue
		}
		if _, err := requireRoleArtifacts(input.Artifacts, spec.Role); err != nil {
			return result, err
		}
	}

	rawReports, rawDegraded, err := pipeline.runRawPhase(ctx, input, result.Failures)
	if err != nil {
		return result, err
	}
	if err := pipeline.requireHealthyPrisms(len(rawReports), PhaseRaw); err != nil {
		result.Status = RoleStatusFailed
		return result, err
	}
	criticalReports, criticalDegraded, err := pipeline.runCriticalPhase(ctx, input, rawReports, result.Failures)
	if err != nil {
		return result, err
	}
	if err := pipeline.requireHealthyPrisms(len(criticalReports), PhaseCritical); err != nil {
		result.Status = RoleStatusFailed
		return result, err
	}
	summaries, summaryDegraded, err := pipeline.runSummaryPhase(ctx, rawReports, criticalReports, input.Artifacts, result.Failures)
	if err != nil {
		return result, err
	}
	if err := pipeline.requireHealthyPrisms(len(summaries), PhaseSummary); err != nil {
		result.Status = RoleStatusFailed
		return result, err
	}

	dialogue := InternalDialogue{Branches: make(map[Prism]PrismDialogue, len(summaries))}
	for _, prism := range allPrisms {
		summary, exists := summaries[prism]
		if !exists {
			continue
		}
		dialogue.Branches[prism] = PrismDialogue{
			Raw:      clonePrismReport(rawReports[prism]),
			Critical: cloneCriticalReport(criticalReports[prism]),
			Summary:  clonePrismSummary(summary),
		}
	}
	result.Dialogue = cloneInternalDialogue(dialogue)
	if len(dialogue.Branches) < len(allPrisms) || rawDegraded || criticalDegraded || summaryDegraded {
		result.Status = RoleStatusDegraded
	}

	toolingArtifacts, _ := requireRoleArtifacts(input.Artifacts, RoleSynthesisTooling)
	toolingOutput, err := pipeline.tooling.RunSynthesisTooling(ctx, SynthesisToolingInput{
		UserInput:   input.UserInput,
		History:     cloneMessages(input.History),
		Context:     cloneContextPack(input.Context),
		Emotion:     input.Emotion,
		Dialogue:    cloneInternalDialogue(dialogue),
		Instruction: toolingArtifacts.Instruction,
		Manifest:    toolingArtifacts.Manifest,
	})
	if err != nil {
		return result, fmt.Errorf("run synthesis tooling: %w", err)
	}
	if strings.TrimSpace(toolingOutput.PreliminaryDecision) == "" {
		return result, errors.New("synthesis tooling returned an empty preliminary decision")
	}
	if err := validateRoleMetadataAgainstArtifacts(toolingOutput.Metadata, RoleSynthesisTooling, toolingArtifacts); err != nil {
		return result, fmt.Errorf("validate synthesis tooling metadata: %w", err)
	}
	result.Tooling = cloneSynthesisToolingOutput(toolingOutput)
	if toolingOutput.Metadata.Status == RoleStatusDegraded {
		result.Status = RoleStatusDegraded
	}

	var toolResults []ToolResult
	if len(toolingOutput.ToolCalls) > 0 {
		if pipeline.executor == nil {
			return result, ErrToolExecutionUnavailable
		}
		toolResults, err = pipeline.executor.ExecuteTools(ctx, cloneToolCalls(toolingOutput.ToolCalls))
		if err != nil {
			return result, fmt.Errorf("execute synthesis tools: %w", err)
		}
	}
	result.ToolResults = cloneToolResults(toolResults)

	finalArtifacts, _ := requireRoleArtifacts(input.Artifacts, RoleSynthesisFinal)
	finalOutput, err := pipeline.final.RunSynthesisFinal(ctx, SynthesisFinalInput{
		Route:               RouteFull,
		UserInput:           input.UserInput,
		History:             cloneMessages(input.History),
		Context:             cloneContextPack(input.Context),
		Emotion:             input.Emotion,
		Dialogue:            cloneInternalDialogue(dialogue),
		PreliminaryDecision: toolingOutput.PreliminaryDecision,
		ToolResults:         cloneToolResults(toolResults),
		Instruction:         finalArtifacts.Instruction,
		Manifest:            finalArtifacts.Manifest,
	})
	if err != nil {
		return result, fmt.Errorf("run synthesis final: %w", err)
	}
	if strings.TrimSpace(finalOutput.Content) == "" {
		return result, errors.New("synthesis final returned empty content")
	}
	if err := validateRoleMetadataAgainstArtifacts(finalOutput.Metadata, RoleSynthesisFinal, finalArtifacts); err != nil {
		return result, fmt.Errorf("validate synthesis final metadata: %w", err)
	}
	result.Final = finalOutput
	if finalOutput.Metadata.Status == RoleStatusDegraded {
		result.Status = RoleStatusDegraded
	}
	return result, nil
}

func (pipeline *FullPipeline) requireHealthyPrisms(count int, phase Phase) error {
	if count < pipeline.options.MinimumHealthyPrisms {
		return fmt.Errorf("%w after %s phase: got %d, need %d", ErrInsufficientPrisms, phase, count, pipeline.options.MinimumHealthyPrisms)
	}
	return nil
}
