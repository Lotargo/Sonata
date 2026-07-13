package emotion

import "time"

// advanceAffectiveRuntimeState advances all elapsed-time dynamics through a
// bounded number of deterministic substeps. It intentionally receives the full
// runtime profile because physiology recovery targets live in the versioned
// initial profile, while emotion, drive and complex-state rules live in the
// dynamics profile.
func advanceAffectiveRuntimeState(
	state *AffectiveState,
	from time.Time,
	elapsed time.Duration,
	profile AffectiveRuntimeProfile,
) (int, error) {
	if elapsed <= 0 {
		return 0, nil
	}

	dynamics := profile.Dynamics
	count := boundedSubstepCount(elapsed, dynamics.IntegrationStep, dynamics.MaxSubsteps)
	base := elapsed / time.Duration(count)
	remainder := elapsed % time.Duration(count)
	cursor := from.UTC()

	for index := 0; index < count; index++ {
		step := base
		if time.Duration(index) < remainder {
			step++
		}

		before := state.Clone()
		advancePhysiology(state, step, profile)
		advanceDrives(state, step, dynamics)
		advanceAffectiveEmotions(state, step, dynamics)
		applyDrivePressure(state, step, dynamics)

		cursor = cursor.Add(step)
		if err := updateComplexStateEvidence(state, before, step, cursor, dynamics); err != nil {
			return 0, err
		}
	}

	return count, nil
}
