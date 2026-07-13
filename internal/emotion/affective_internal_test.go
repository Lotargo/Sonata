package emotion

import (
	"testing"
	"time"
)

func TestFatigueRecoversTowardVersionedBaseline(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	state.Physiology.Fatigue = 1
	state.Physiology.Energy = 0.2

	next, _, err := Transition(state, nil, start.Add(12*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if !(next.Physiology.Fatigue < state.Physiology.Fatigue && next.Physiology.Fatigue > profile.Initial.Physiology.Fatigue) {
		t.Fatalf("fatigue did not recover toward baseline: before=%f after=%f baseline=%f", state.Physiology.Fatigue, next.Physiology.Fatigue, profile.Initial.Physiology.Fatigue)
	}
	if !(next.Physiology.Energy > state.Physiology.Energy && next.Physiology.Energy < profile.Initial.Physiology.Energy) {
		t.Fatalf("energy did not recover toward baseline: before=%f after=%f baseline=%f", state.Physiology.Energy, next.Physiology.Energy, profile.Initial.Physiology.Energy)
	}
}

func TestDepressiveStateSlowsFatigueRecovery(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	state.Physiology.Fatigue = 1
	depressive := profile.Dynamics.ComplexStates[1]

	normal, _, err := Transition(state, nil, start.Add(6*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}

	depressedState := state.Clone()
	depressedState.ComplexStates = []ComplexState{{
		Kind:         ComplexStateDepressive,
		DefinitionID: depressive.DefinitionID,
		Activation:   1,
		ActiveSince:  start.Add(-15 * 24 * time.Hour),
	}}
	depressed, _, err := Transition(depressedState, nil, start.Add(6*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if depressed.Physiology.Fatigue <= normal.Physiology.Fatigue {
		t.Fatalf("depressive state did not slow fatigue recovery: depressed=%f normal=%f", depressed.Physiology.Fatigue, normal.Physiology.Fatigue)
	}
}

func TestUnsatisfiedDriveCreatesBoundedEmotionPressure(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	for index := range state.Drives {
		if state.Drives[index].Kind == DriveSafety {
			state.Drives[index].Satisfaction = 0
			state.Drives[index].Urgency = 0
		}
	}
	beforeFear := state.Emotions.Fear

	next, log, err := Transition(state, nil, start.Add(24*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	safety := driveByKind(t, next.Drives, DriveSafety)
	if safety.Urgency <= 0 {
		t.Fatal("unsatisfied safety drive did not develop urgency")
	}
	if next.Emotions.Fear <= beforeFear {
		t.Fatalf("safety pressure did not increase fear: before=%f after=%f", beforeFear, next.Emotions.Fear)
	}
	if log.IntegrationSubsteps > profile.Dynamics.MaxSubsteps {
		t.Fatalf("drive pressure exceeded bounded substeps: %d", log.IntegrationSubsteps)
	}
	if err := next.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSatisfyingStimulusCanReduceUrgencyImmediately(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	for index := range state.Drives {
		if state.Drives[index].Kind == DriveSafety {
			state.Drives[index].Satisfaction = 0.2
			state.Drives[index].Urgency = 0.7
		}
	}
	before := driveByKind(t, state.Drives, DriveSafety)
	stimulus := Stimulus{
		Kind:       StimulusUserTrust,
		Source:     "test",
		Intensity:  1,
		Confidence: 1,
		Valence:    1,
		CreatedAt:  start,
	}

	next, _, err := Transition(state, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	after := driveByKind(t, next.Drives, DriveSafety)
	if after.Satisfaction <= before.Satisfaction || after.Urgency >= before.Urgency {
		t.Fatalf("satisfying stimulus did not reduce urgency: before=%#v after=%#v", before, after)
	}
}
