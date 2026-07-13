package cognition

import (
	"context"
	"errors"
)

type PipelineInput struct {
	UserInput string
	History   []ConversationMessage
	Context   ContextPack
	Emotion   EmotionReport
	Artifacts map[RuntimeRole]RoleArtifacts
}

type PipelineResult struct {
	Route  Route
	Status RoleStatus
	Direct *DirectPipelineResult
	Full   *FullPipelineResult
}

type Pipeline struct {
	direct *DirectPipeline
	full   *FullPipeline
}

func NewPipeline(
	router RouterRunner,
	raw RawRunner,
	critical CriticalRunner,
	summary SummaryRunner,
	tooling SynthesisToolingRunner,
	final SynthesisFinalRunner,
	executor ToolExecutor,
	options FullPipelineOptions,
) (*Pipeline, error) {
	direct, err := NewDirectPipeline(router, final)
	if err != nil {
		return nil, err
	}
	full, err := NewFullPipeline(raw, critical, summary, tooling, final, executor, options)
	if err != nil {
		return nil, err
	}
	return &Pipeline{direct: direct, full: full}, nil
}

func (pipeline *Pipeline) Run(ctx context.Context, input PipelineInput) (PipelineResult, error) {
	if pipeline == nil || pipeline.direct == nil || pipeline.full == nil {
		return PipelineResult{}, errors.New("cognitive pipeline is not initialized")
	}
	finalArtifacts, err := requireRoleArtifacts(input.Artifacts, RoleSynthesisFinal)
	if err != nil {
		return PipelineResult{}, err
	}
	directResult, err := pipeline.direct.Run(ctx, DirectPipelineInput{
		UserInput:        input.UserInput,
		History:          cloneMessages(input.History),
		FinalInstruction: finalArtifacts.Instruction,
		FinalManifest:    finalArtifacts.Manifest,
	})
	if err == nil {
		status := directResult.Final.Metadata.Status
		if directResult.Router.Status == RoleStatusDegraded {
			status = RoleStatusDegraded
		}
		return PipelineResult{
			Route:  RouteDirect,
			Status: status,
			Direct: &directResult,
		}, nil
	}
	if !errors.Is(err, ErrFullRouteRequired) {
		return PipelineResult{}, err
	}
	fullResult, err := pipeline.full.RunAfterRouter(ctx, FullPipelineInput{
		UserInput: input.UserInput,
		History:   cloneMessages(input.History),
		Context:   cloneContextPack(input.Context),
		Emotion:   input.Emotion,
		Router:    directResult.Router,
		Artifacts: cloneRoleArtifacts(input.Artifacts),
	})
	result := PipelineResult{
		Route:  RouteFull,
		Full:   &fullResult,
		Status: fullResult.Status,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func cloneRoleArtifacts(values map[RuntimeRole]RoleArtifacts) map[RuntimeRole]RoleArtifacts {
	cloned := make(map[RuntimeRole]RoleArtifacts, len(values))
	for role, artifacts := range values {
		cloned[role] = artifacts
	}
	return cloned
}
