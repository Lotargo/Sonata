package cognition

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrFullRouteRequired = errors.New("full cognitive route required")

type RouterRunResult struct {
	Output   RouterOutput
	Metadata RoleMetadata
}

type RouterRunner interface {
	RunRouter(context.Context, RouterInput) (RouterRunResult, error)
}

type SynthesisFinalRunner interface {
	RunSynthesisFinal(context.Context, SynthesisFinalInput) (SynthesisFinalOutput, error)
}

type DirectPipelineInput struct {
	UserInput        string
	History          []ConversationMessage
	FinalInstruction ArtifactRef
	FinalManifest    ManifestRef
}

type DirectPipelineResult struct {
	Route  Route
	Router RoleMetadata
	Final  SynthesisFinalOutput
}

type DirectPipeline struct {
	router RouterRunner
	final  SynthesisFinalRunner
}

func NewDirectPipeline(router RouterRunner, final SynthesisFinalRunner) (*DirectPipeline, error) {
	if router == nil {
		return nil, errors.New("router runner is required")
	}
	if final == nil {
		return nil, errors.New("synthesis final runner is required")
	}
	return &DirectPipeline{router: router, final: final}, nil
}

func (pipeline *DirectPipeline) Run(ctx context.Context, input DirectPipelineInput) (DirectPipelineResult, error) {
	if pipeline == nil || pipeline.router == nil || pipeline.final == nil {
		return DirectPipelineResult{}, errors.New("direct pipeline is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return DirectPipelineResult{}, err
	}
	if strings.TrimSpace(input.UserInput) == "" {
		return DirectPipelineResult{}, errors.New("direct pipeline user input is required")
	}
	finalArtifacts := RoleArtifacts{Instruction: input.FinalInstruction, Manifest: input.FinalManifest}
	if err := validateRoleArtifacts(RoleSynthesisFinal, finalArtifacts); err != nil {
		return DirectPipelineResult{}, err
	}

	routerResult, err := pipeline.router.RunRouter(ctx, RouterInput{
		UserInput: input.UserInput,
		History:   cloneMessages(input.History),
	})
	if err != nil {
		return DirectPipelineResult{}, fmt.Errorf("run router: %w", err)
	}
	if err := routerResult.Output.Validate(); err != nil {
		return DirectPipelineResult{}, fmt.Errorf("validate router output: %w", err)
	}
	if err := validateRoleMetadata(routerResult.Metadata, RoleRouter); err != nil {
		return DirectPipelineResult{}, fmt.Errorf("validate router metadata: %w", err)
	}

	result := DirectPipelineResult{Route: routerResult.Output.Route, Router: routerResult.Metadata}
	if routerResult.Output.Route == RouteFull {
		return result, ErrFullRouteRequired
	}

	final, err := pipeline.final.RunSynthesisFinal(ctx, SynthesisFinalInput{
		Route:       RouteDirect,
		UserInput:   input.UserInput,
		History:     cloneMessages(input.History),
		Instruction: input.FinalInstruction,
		Manifest:    input.FinalManifest,
	})
	if err != nil {
		return result, fmt.Errorf("run synthesis final: %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return result, errors.New("synthesis final returned empty content")
	}
	if err := validateRoleMetadataAgainstArtifacts(final.Metadata, RoleSynthesisFinal, finalArtifacts); err != nil {
		return result, fmt.Errorf("validate synthesis final metadata: %w", err)
	}
	result.Final = final
	return result, nil
}

func validateArtifactRef(ref ArtifactRef, name string) error {
	if strings.TrimSpace(ref.ID) == "" || ref.Version < 1 || strings.TrimSpace(ref.Hash) == "" {
		return fmt.Errorf("%s metadata is incomplete", name)
	}
	return nil
}

func validateManifestRef(ref ManifestRef) error {
	if err := validateArtifactRef(ref.ArtifactRef, "manifest"); err != nil {
		return err
	}
	if strings.TrimSpace(ref.Source) == "" {
		return errors.New("synthesis final manifest source is required")
	}
	return nil
}

func validateRoleMetadata(metadata RoleMetadata, expected RuntimeRole) error {
	if metadata.Role != expected {
		return fmt.Errorf("role metadata is %q, want %q", metadata.Role, expected)
	}
	if metadata.Status != RoleStatusSucceeded && metadata.Status != RoleStatusDegraded {
		return fmt.Errorf("role %s has invalid successful status %q", expected, metadata.Status)
	}
	if strings.TrimSpace(metadata.ModelID) == "" {
		return fmt.Errorf("role %s model ID is required", expected)
	}
	if metadata.Latency < 0 {
		return fmt.Errorf("role %s latency cannot be negative", expected)
	}
	spec, exists := RoleSpecFor(expected)
	if !exists {
		return fmt.Errorf("unknown runtime role %q", expected)
	}
	if err := validateArtifactRef(metadata.Instruction, "role instruction"); err != nil {
		return err
	}
	if metadata.Instruction.ID != spec.InstructionID {
		return fmt.Errorf("role %s instruction ID is %q, want %q", expected, metadata.Instruction.ID, spec.InstructionID)
	}
	if err := validateManifestRef(metadata.Manifest); err != nil {
		return err
	}
	return nil
}

func cloneMessages(messages []ConversationMessage) []ConversationMessage {
	return append([]ConversationMessage(nil), messages...)
}
