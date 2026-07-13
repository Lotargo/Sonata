package emotion

import (
	"strings"
	"testing"
)

func TestTransitionRejectsMismatchedComplexStateDefinition(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	state.ComplexStates = []ComplexState{{
		Kind:         ComplexStateDepressive,
		DefinitionID: "stale-depressive-definition",
		Activation:   1,
		ActiveSince:  start,
	}}

	_, _, err := Transition(state, nil, start, profile)
	if err == nil || !strings.Contains(err.Error(), "does not match profile definition") {
		t.Fatalf("expected stale complex-state definition error, got %v", err)
	}
}

func TestPartialComplexStateActivationInterpolatesEffects(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	depressive := profile.Dynamics.ComplexStates[1]
	stimulus := Stimulus{
		Kind:       StimulusUserSuccess,
		Source:     "test",
		Intensity:  1,
		Confidence: 1,
		Valence:    1,
		Arousal:    0.5,
		CreatedAt:  start,
	}

	normal, _, err := Transition(state, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	partialState := state.Clone()
	partialState.ComplexStates = []ComplexState{{
		Kind:         ComplexStateDepressive,
		DefinitionID: depressive.DefinitionID,
		Activation:   0.5,
		ActiveSince:  start,
	}}
	partial, _, err := Transition(partialState, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	fullState := state.Clone()
	fullState.ComplexStates = []ComplexState{{
		Kind:         ComplexStateDepressive,
		DefinitionID: depressive.DefinitionID,
		Activation:   1,
		ActiveSince:  start,
	}}
	full, _, err := Transition(fullState, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	if !(full.Emotions.Joy < partial.Emotions.Joy && partial.Emotions.Joy < normal.Emotions.Joy) {
		t.Fatalf("activation was not interpolated: full=%f partial=%f normal=%f", full.Emotions.Joy, partial.Emotions.Joy, normal.Emotions.Joy)
	}
	if got := interpolateMultiplier(0.2, 0.5); got != 0.6 {
		t.Fatalf("interpolated multiplier = %f, want 0.6", got)
	}
}
