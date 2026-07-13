package emotion

import (
	"testing"
	"time"
)

func TestBaselineAffectiveStateIsValidAndDeterministic(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	state, err := NewBaselineAffectiveState(
		StateKey{IdentityID: "sonata", UserID: "user-1"},
		"affective-v1-test",
		Vector{
			Joy: 0.35, Trust: 0.45, Fear: 0.10, Surprise: 0.15,
			Sadness: 0.10, Disgust: 0.05, Anger: 0.05, Anticipation: 0.30,
		},
		RelationshipState{
			Openness:         0.5,
			ConfidenceInUser: 0.5,
			PerceivedSafety:  0.5,
		},
		at,
	)
	if err != nil {
		t.Fatalf("create baseline affective state: %v", err)
	}
	if state.Version != 0 || state.LastUpdatedAt != at {
		t.Fatalf("unexpected state metadata: %#v", state)
	}
	if len(state.Drives) != len(AllDriveKinds()) {
		t.Fatalf("drive count = %d, want %d", len(state.Drives), len(AllDriveKinds()))
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate baseline affective state: %v", err)
	}
}

func TestAffectiveStateCloneDoesNotShareSliceStorage(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	state, err := NewBaselineAffectiveState(
		StateKey{IdentityID: "sonata", UserID: "user-1"},
		"affective-v1-test",
		Vector{},
		RelationshipState{},
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	state.ComplexStates = []ComplexState{{
		Kind:        ComplexStateChronicStress,
		Activation:  Unit(0.5),
		ActiveSince: at,
	}}
	state.Evidence = []StateEvidence{{
		Kind: ComplexStateChronicStress,
		Evidence: EvidenceAccumulator{
			PositiveArea:  NonNegative(2),
			ObservedFor:   time.Hour,
			LastUpdatedAt: at,
		},
	}}

	clone := state.Clone()
	clone.Drives[0].Urgency = 1
	clone.ComplexStates[0].Activation = 1
	clone.Evidence[0].Evidence.PositiveArea = 10

	if state.Drives[0].Urgency == clone.Drives[0].Urgency {
		t.Fatal("drive slice storage is shared")
	}
	if state.ComplexStates[0].Activation == clone.ComplexStates[0].Activation {
		t.Fatal("complex-state slice storage is shared")
	}
	if state.Evidence[0].Evidence.PositiveArea == clone.Evidence[0].Evidence.PositiveArea {
		t.Fatal("evidence slice storage is shared")
	}
}

func TestAffectiveStateRequiresProfileVersionAndOwner(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	_, err := NewBaselineAffectiveState(
		StateKey{IdentityID: "sonata", UserID: ""},
		"",
		Vector{},
		RelationshipState{},
		at,
	)
	if err == nil {
		t.Fatal("expected invalid owner and profile version to be rejected")
	}
}
