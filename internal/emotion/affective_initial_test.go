package emotion

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

func TestBaselineStateIsDerivedFromVersionedProfiles(t *testing.T) {
	setAffectiveTestSecrets(t)

	root := filepath.Join("..", "..", "config")
	runtimeConfig, err := config.NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatalf("load repository config: %v", err)
	}
	dynamics, err := NewAffectiveProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := NewAffectiveInitialProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.July, 13, 18, 0, 0, 0, time.UTC)
	state, err := NewBaselineAffectiveStateFromProfiles(
		StateKey{IdentityID: "sonata", UserID: "user-1"},
		dynamics,
		initial,
		RelationshipState{Openness: 0.5, ConfidenceInUser: 0.5, PerceivedSafety: 0.5},
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	if state.ProfileVersion != dynamics.Version || state.Emotions.Joy != dynamics.Dynamics[0].Baseline.Float64() {
		t.Fatalf("state was not derived from profile: %#v", state)
	}
	if state.Physiology != initial.Physiology {
		t.Fatalf("physiology = %#v, want %#v", state.Physiology, initial.Physiology)
	}
	if len(state.Drives) != len(initial.Drives) {
		t.Fatalf("drive count = %d, want %d", len(state.Drives), len(initial.Drives))
	}
	for index, drive := range state.Drives {
		initialDrive := initial.Drives[index]
		if drive.Kind != initialDrive.Kind || drive.Level != initialDrive.Level || drive.Satisfaction != initialDrive.Satisfaction || drive.Urgency != initialDrive.Urgency {
			t.Fatalf("drive[%d] = %#v, want %#v", index, drive, initialDrive)
		}
	}
}

func TestBaselineStateRejectsMismatchedProfileVersions(t *testing.T) {
	setAffectiveTestSecrets(t)

	root := filepath.Join("..", "..", "config")
	runtimeConfig, err := config.NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatal(err)
	}
	dynamics, err := NewAffectiveProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := NewAffectiveInitialProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatal(err)
	}
	initial.ProfileVersion = "other-version"
	_, err = NewBaselineAffectiveStateFromProfiles(
		StateKey{IdentityID: "sonata", UserID: "user-1"},
		dynamics,
		initial,
		RelationshipState{},
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected mismatched profile versions to be rejected")
	}
}
