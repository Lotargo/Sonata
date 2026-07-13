package cognition

import (
	"context"
	"sync"
)

func (pipeline *FullPipeline) runRawPhase(ctx context.Context, input FullPipelineInput, failures map[Prism]BranchFailure) (map[Prism]PrismReport, bool, error) {
	outcomes := runPrismPhase(ctx, pipeline.options, AllPrisms(), func(callCtx context.Context, prism Prism) (PrismReport, error) {
		role := roleForPrismPhase(prism, PhaseRaw)
		artifacts, _ := requireRoleArtifacts(input.Artifacts, role)
		return pipeline.raw.RunRaw(callCtx, RawInput{
			Prism:       prism,
			UserInput:   input.UserInput,
			History:     cloneMessages(input.History),
			Context:     cloneContextPack(input.Context),
			Emotion:     input.Emotion,
			Instruction: artifacts.Instruction,
			Manifest:    artifacts.Manifest,
		})
	})
	reports := make(map[Prism]PrismReport, len(outcomes))
	degraded := false
	for _, prism := range allPrisms {
		outcome := outcomes[prism]
		if outcome.Err != nil {
			failures[prism] = BranchFailure{Phase: PhaseRaw, Err: outcome.Err}
			continue
		}
		role := roleForPrismPhase(prism, PhaseRaw)
		artifacts, _ := requireRoleArtifacts(input.Artifacts, role)
		if err := validatePrismReport(outcome.Value, prism, role, artifacts); err != nil {
			failures[prism] = BranchFailure{Phase: PhaseRaw, Err: err}
			continue
		}
		if outcome.Value.Metadata.Status == RoleStatusDegraded {
			degraded = true
		}
		reports[prism] = clonePrismReport(outcome.Value)
	}
	if err := ctx.Err(); err != nil {
		return reports, degraded, err
	}
	return reports, degraded, nil
}

func (pipeline *FullPipeline) runCriticalPhase(ctx context.Context, input FullPipelineInput, raw map[Prism]PrismReport, failures map[Prism]BranchFailure) (map[Prism]CriticalReport, bool, error) {
	prisms := availablePrisms(raw)
	outcomes := runPrismPhase(ctx, pipeline.options, prisms, func(callCtx context.Context, prism Prism) (CriticalReport, error) {
		role := roleForPrismPhase(prism, PhaseCritical)
		artifacts, _ := requireRoleArtifacts(input.Artifacts, role)
		return pipeline.critical.RunCritical(callCtx, CriticalInput{
			Prism:       prism,
			UserInput:   input.UserInput,
			Context:     cloneContextPack(input.Context),
			Emotion:     input.Emotion,
			Raw:         clonePrismReport(raw[prism]),
			Instruction: artifacts.Instruction,
			Manifest:    artifacts.Manifest,
		})
	})
	reports := make(map[Prism]CriticalReport, len(outcomes))
	degraded := false
	for _, prism := range prisms {
		outcome := outcomes[prism]
		if outcome.Err != nil {
			failures[prism] = BranchFailure{Phase: PhaseCritical, Err: outcome.Err}
			continue
		}
		role := roleForPrismPhase(prism, PhaseCritical)
		artifacts, _ := requireRoleArtifacts(input.Artifacts, role)
		if err := validateCriticalReport(outcome.Value, prism, role, artifacts); err != nil {
			failures[prism] = BranchFailure{Phase: PhaseCritical, Err: err}
			continue
		}
		if outcome.Value.Metadata.Status == RoleStatusDegraded {
			degraded = true
		}
		reports[prism] = cloneCriticalReport(outcome.Value)
	}
	if err := ctx.Err(); err != nil {
		return reports, degraded, err
	}
	return reports, degraded, nil
}

func (pipeline *FullPipeline) runSummaryPhase(ctx context.Context, raw map[Prism]PrismReport, critical map[Prism]CriticalReport, artifacts map[RuntimeRole]RoleArtifacts, failures map[Prism]BranchFailure) (map[Prism]PrismSummary, bool, error) {
	prisms := availablePrisms(critical)
	outcomes := runPrismPhase(ctx, pipeline.options, prisms, func(callCtx context.Context, prism Prism) (PrismSummary, error) {
		role := roleForPrismPhase(prism, PhaseSummary)
		roleArtifacts, _ := requireRoleArtifacts(artifacts, role)
		return pipeline.summary.RunSummary(callCtx, SummaryInput{
			Prism:       prism,
			Raw:         clonePrismReport(raw[prism]),
			Critical:    cloneCriticalReport(critical[prism]),
			Instruction: roleArtifacts.Instruction,
			Manifest:    roleArtifacts.Manifest,
		})
	})
	summaries := make(map[Prism]PrismSummary, len(outcomes))
	degraded := false
	for _, prism := range prisms {
		outcome := outcomes[prism]
		if outcome.Err != nil {
			failures[prism] = BranchFailure{Phase: PhaseSummary, Err: outcome.Err}
			continue
		}
		role := roleForPrismPhase(prism, PhaseSummary)
		roleArtifacts, _ := requireRoleArtifacts(artifacts, role)
		if err := validatePrismSummary(outcome.Value, prism, role, roleArtifacts); err != nil {
			failures[prism] = BranchFailure{Phase: PhaseSummary, Err: err}
			continue
		}
		if outcome.Value.Metadata.Status == RoleStatusDegraded {
			degraded = true
		}
		summaries[prism] = clonePrismSummary(outcome.Value)
	}
	if err := ctx.Err(); err != nil {
		return summaries, degraded, err
	}
	return summaries, degraded, nil
}

type prismPhaseOutcome[T any] struct {
	Prism Prism
	Value T
	Err   error
}

func runPrismPhase[T any](ctx context.Context, options FullPipelineOptions, prisms []Prism, run func(context.Context, Prism) (T, error)) map[Prism]prismPhaseOutcome[T] {
	phaseCtx, cancel := context.WithTimeout(ctx, options.PhaseTimeout)
	defer cancel()
	semaphore := make(chan struct{}, options.MaxConcurrentPrisms)
	results := make(chan prismPhaseOutcome[T], len(prisms))
	var wait sync.WaitGroup
	for _, prism := range prisms {
		prism := prism
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-phaseCtx.Done():
				results <- prismPhaseOutcome[T]{Prism: prism, Err: phaseCtx.Err()}
				return
			}
			value, err := run(phaseCtx, prism)
			results <- prismPhaseOutcome[T]{Prism: prism, Value: value, Err: err}
		}()
	}
	wait.Wait()
	close(results)
	outcomes := make(map[Prism]prismPhaseOutcome[T], len(prisms))
	for outcome := range results {
		outcomes[outcome.Prism] = outcome
	}
	return outcomes
}
