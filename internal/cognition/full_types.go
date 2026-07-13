package cognition

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInsufficientPrisms       = errors.New("insufficient healthy prism branches")
	ErrToolExecutionUnavailable = errors.New("tool execution is unavailable")
)

type RawRunner interface {
	RunRaw(context.Context, RawInput) (PrismReport, error)
}

type CriticalRunner interface {
	RunCritical(context.Context, CriticalInput) (CriticalReport, error)
}

type SummaryRunner interface {
	RunSummary(context.Context, SummaryInput) (PrismSummary, error)
}

type SynthesisToolingRunner interface {
	RunSynthesisTooling(context.Context, SynthesisToolingInput) (SynthesisToolingOutput, error)
}

type ToolExecutor interface {
	ExecuteTools(context.Context, []ToolCall) ([]ToolResult, error)
}

type RoleArtifacts struct {
	Instruction ArtifactRef
	Manifest    ManifestRef
}

type FullPipelineOptions struct {
	PhaseTimeout         time.Duration
	MaxConcurrentPrisms  int
	MinimumHealthyPrisms int
}

type FullPipelineInput struct {
	UserInput string
	History   []ConversationMessage
	Context   ContextPack
	Emotion   EmotionReport
	Router    RoleMetadata
	Artifacts map[RuntimeRole]RoleArtifacts
}

type BranchFailure struct {
	Phase Phase
	Err   error
}

type FullPipelineResult struct {
	Status      RoleStatus
	Router      RoleMetadata
	Dialogue    InternalDialogue
	Tooling     SynthesisToolingOutput
	ToolResults []ToolResult
	Final       SynthesisFinalOutput
	Failures    map[Prism]BranchFailure
}

type FullPipeline struct {
	raw      RawRunner
	critical CriticalRunner
	summary  SummaryRunner
	tooling  SynthesisToolingRunner
	final    SynthesisFinalRunner
	executor ToolExecutor
	options  FullPipelineOptions
}

func NewFullPipeline(
	raw RawRunner,
	critical CriticalRunner,
	summary SummaryRunner,
	tooling SynthesisToolingRunner,
	final SynthesisFinalRunner,
	executor ToolExecutor,
	options FullPipelineOptions,
) (*FullPipeline, error) {
	if raw == nil || critical == nil || summary == nil || tooling == nil || final == nil {
		return nil, errors.New("all full pipeline role runners are required")
	}
	if options.PhaseTimeout < 0 {
		return nil, errors.New("phase timeout cannot be negative")
	}
	if options.PhaseTimeout == 0 {
		options.PhaseTimeout = 2 * time.Minute
	}
	if options.MaxConcurrentPrisms < 0 {
		return nil, errors.New("max concurrent prisms cannot be negative")
	}
	if options.MaxConcurrentPrisms == 0 {
		options.MaxConcurrentPrisms = len(allPrisms)
	}
	if options.MaxConcurrentPrisms > len(allPrisms) {
		return nil, fmt.Errorf("max concurrent prisms cannot exceed %d", len(allPrisms))
	}
	if options.MinimumHealthyPrisms < 0 {
		return nil, errors.New("minimum healthy prisms cannot be negative")
	}
	if options.MinimumHealthyPrisms == 0 {
		options.MinimumHealthyPrisms = len(allPrisms) - 1
	}
	if options.MinimumHealthyPrisms > len(allPrisms) {
		return nil, fmt.Errorf("minimum healthy prisms cannot exceed %d", len(allPrisms))
	}
	return &FullPipeline{
		raw: raw, critical: critical, summary: summary, tooling: tooling, final: final,
		executor: executor, options: options,
	}, nil
}
