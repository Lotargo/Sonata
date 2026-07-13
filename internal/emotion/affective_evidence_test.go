package emotion

import (
	"testing"
	"time"
)

func TestTemporalEvidenceEntersAndExitsWithHysteresis(t *testing.T) {
	profile, state, start := loadEvidenceFixture(t)

	entered, log, err := Transition(state, nil, start.Add(75*time.Minute), profile)
	if err != nil {
		t.Fatal(err)
	}
	if log.IntegrationSubsteps != 5 {
		t.Fatalf("integration substeps = %d, want 5", log.IntegrationSubsteps)
	}
	index := complexStateIndex(entered.ComplexStates, ComplexStateDepressive)
	if index < 0 {
		t.Fatalf("depressive state did not enter: %#v", entered.ComplexStates)
	}
	if entered.ComplexStates[index].ActiveSince != start.Add(time.Hour) {
		t.Fatalf("active since = %v, want %v", entered.ComplexStates[index].ActiveSince, start.Add(time.Hour))
	}

	entered.Emotions.Sadness = 0.1
	exited, _, err := Transition(entered, nil, entered.LastUpdatedAt.Add(time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if complexStateIndex(exited.ComplexStates, ComplexStateDepressive) >= 0 {
		t.Fatal("depressive state did not exit after sustained exit evidence")
	}
}

func TestTemporalEvidenceViolationResetsConsecutiveDuration(t *testing.T) {
	profile, state, start := loadEvidenceFixture(t)

	first, _, err := Transition(state, nil, start.Add(30*time.Minute), profile)
	if err != nil {
		t.Fatal(err)
	}
	first.Emotions.Sadness = 0.1
	broken, _, err := Transition(first, nil, first.LastUpdatedAt.Add(15*time.Minute), profile)
	if err != nil {
		t.Fatal(err)
	}
	broken.Emotions.Sadness = 0.8
	almost, _, err := Transition(broken, nil, broken.LastUpdatedAt.Add(45*time.Minute), profile)
	if err != nil {
		t.Fatal(err)
	}
	if complexStateIndex(almost.ComplexStates, ComplexStateDepressive) >= 0 {
		t.Fatal("complex state entered without a consecutive qualifying duration")
	}
	entered, _, err := Transition(almost, nil, almost.LastUpdatedAt.Add(15*time.Minute), profile)
	if err != nil {
		t.Fatal(err)
	}
	if complexStateIndex(entered.ComplexStates, ComplexStateDepressive) < 0 {
		t.Fatal("complex state did not enter after a renewed full qualifying duration")
	}
}

func TestTemporalEvidenceSubstepsAreBounded(t *testing.T) {
	profile, state, start := loadEvidenceFixture(t)

	next, log, err := Transition(state, nil, start.Add(10*365*24*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if log.IntegrationSubsteps != profile.Dynamics.MaxSubsteps {
		t.Fatalf("integration substeps = %d, want bounded maximum %d", log.IntegrationSubsteps, profile.Dynamics.MaxSubsteps)
	}
	if err := next.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTemporalEvidenceUsesConservativeIntervalScore(t *testing.T) {
	profile, state, start := loadEvidenceFixture(t)
	state.Emotions.Sadness = 0.8
	profile.Dynamics.Dynamics[emotionIndex(Sadness)].RecoveryRate = 2

	next, _, err := Transition(state, nil, start.Add(time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if complexStateIndex(next.ComplexStates, ComplexStateDepressive) >= 0 {
		t.Fatal("state entered even though the condition stopped holding during the interval")
	}
}

func loadEvidenceFixture(t *testing.T) (AffectiveRuntimeProfile, AffectiveState, time.Time) {
	t.Helper()
	profile, state, start := loadTransitionFixture(t)
	sadnessSignal, err := ParseConditionSignal("emotion.sadness")
	if err != nil {
		t.Fatal(err)
	}
	for index := range profile.Dynamics.ComplexStates {
		definition := &profile.Dynamics.ComplexStates[index]
		definition.MinEntryDuration = time.Hour
		definition.MinExitDuration = time.Hour
		if definition.Kind == ComplexStateDepressive {
			definition.EntryConditions = []Condition{{
				Signal: sadnessSignal, Operator: ConditionGTE, Threshold: 0.7, Weight: 1,
			}}
			definition.ExitConditions = []Condition{{
				Signal: sadnessSignal, Operator: ConditionLTE, Threshold: 0.3, Weight: 1,
			}}
			definition.EntryThreshold = 1
			definition.ExitThreshold = 0.9
		} else {
			definition.EntryConditions = []Condition{{
				Signal: sadnessSignal, Operator: ConditionGTE, Threshold: 1, Weight: 1,
			}}
			definition.ExitConditions = []Condition{{
				Signal: sadnessSignal, Operator: ConditionLTE, Threshold: 0, Weight: 1,
			}}
			definition.EntryThreshold = 1
			definition.ExitThreshold = 0.9
		}
	}
	for index := range profile.Dynamics.Dynamics {
		profile.Dynamics.Dynamics[index].RecoveryRate = 0
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("validate evidence test profile: %v", err)
	}
	state.Emotions.Sadness = 0.8
	return profile, state, start
}
