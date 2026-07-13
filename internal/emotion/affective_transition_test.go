package emotion

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

func TestTransitionSortsTimelineAndIsDeterministic(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	warmth := Stimulus{
		Kind: StimulusUserWarmth, Source: "test", Intensity: 0.8, Confidence: 1,
		Valence: 0.8, Arousal: 0.2, CreatedAt: start.Add(time.Hour),
	}
	hostility := Stimulus{
		Kind: StimulusUserHostility, Source: "test", Intensity: 0.6, Confidence: 0.9,
		Valence: -1, Arousal: 0.8, CreatedAt: start.Add(2 * time.Hour),
	}

	first, firstLog, err := Transition(state, []Stimulus{hostility, warmth}, start.Add(3*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	second, secondLog, err := Transition(state, []Stimulus{warmth, hostility}, start.Add(3*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstLog, secondLog) {
		t.Fatalf("event order changed deterministic result:\nfirst=%#v %#v\nsecond=%#v %#v", first, firstLog, second, secondLog)
	}
	if first.Version != state.Version+1 || firstLog.RecoverySegments != 3 || firstLog.AppliedStimuli != 2 {
		t.Fatalf("unexpected transition metadata: state=%#v log=%#v", first, firstLog)
	}
	if len(firstLog.StimulusDefinitions) != 2 || firstLog.StimulusDefinitions[0] != "stimulus-user-warmth-v1" {
		t.Fatalf("stimulus definitions = %#v", firstLog.StimulusDefinitions)
	}
}

func TestPersonalityAndFatigueChangeJoyResponse(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	stimulus := Stimulus{
		Kind: StimulusUserSuccess, Source: "test", Intensity: 1, Confidence: 1,
		Valence: 1, Arousal: 0.5, CreatedAt: start,
	}

	highExtraversion := profile
	highExtraversion.Dynamics.Personality.Extraversion = 1
	lowExtraversion := profile
	lowExtraversion.Dynamics.Personality.Extraversion = 0
	high, _, err := Transition(state, []Stimulus{stimulus}, start, highExtraversion)
	if err != nil {
		t.Fatal(err)
	}
	low, _, err := Transition(state, []Stimulus{stimulus}, start, lowExtraversion)
	if err != nil {
		t.Fatal(err)
	}
	if high.Emotions.Joy <= low.Emotions.Joy {
		t.Fatalf("extraversion did not increase joy response: high=%f low=%f", high.Emotions.Joy, low.Emotions.Joy)
	}

	fatigued := state.Clone()
	fatigued.Physiology.Fatigue = 1
	fatiguedResult, _, err := Transition(fatigued, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	baselineResult, _, err := Transition(state, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	if fatiguedResult.Emotions.Joy >= baselineResult.Emotions.Joy {
		t.Fatalf("fatigue did not suppress joy response: fatigued=%f baseline=%f", fatiguedResult.Emotions.Joy, baselineResult.Emotions.Joy)
	}
}

func TestDepressiveStateSuppressesJoyAndRaisesRecoveryUrgency(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	depressive := profile.Dynamics.ComplexStates[1]
	state.ComplexStates = []ComplexState{{
		Kind:         ComplexStateDepressive,
		DefinitionID: depressive.DefinitionID,
		Activation:   1,
		ActiveSince:  start.Add(-15 * 24 * time.Hour),
	}}
	stimulus := Stimulus{
		Kind: StimulusUserSuccess, Source: "test", Intensity: 1, Confidence: 1,
		Valence: 1, Arousal: 0.5, CreatedAt: start,
	}
	depressed, _, err := Transition(state, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	normal := state.Clone()
	normal.ComplexStates = nil
	normalResult, _, err := Transition(normal, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	if depressed.Emotions.Joy >= normalResult.Emotions.Joy {
		t.Fatalf("depressive state did not suppress joy: depressed=%f normal=%f", depressed.Emotions.Joy, normalResult.Emotions.Joy)
	}
	if depressed.Emotions.Joy > 0.38+1e-9 {
		t.Fatalf("depressive joy ceiling exceeded: %f", depressed.Emotions.Joy)
	}
	if driveByKind(t, depressed.Drives, DriveRecovery).Urgency <= driveByKind(t, normalResult.Drives, DriveRecovery).Urgency {
		t.Fatal("depressive state did not increase recovery urgency")
	}
}

func TestHostilityUpdatesPhysiologyRelationshipAndSafetyDrive(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	beforeSafety := driveByKind(t, state.Drives, DriveSafety)
	stimulus := Stimulus{
		Kind: StimulusUserHostility, Source: "test", Intensity: 1, Confidence: 1,
		Valence: -1, Arousal: 1, CreatedAt: start,
	}
	next, _, err := Transition(state, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	afterSafety := driveByKind(t, next.Drives, DriveSafety)
	if next.Physiology.StressLoad <= state.Physiology.StressLoad || next.Physiology.Stability >= state.Physiology.Stability {
		t.Fatalf("hostility physiology effects missing: before=%#v after=%#v", state.Physiology, next.Physiology)
	}
	if next.Relationship.Tension <= state.Relationship.Tension || next.Relationship.PerceivedSafety >= state.Relationship.PerceivedSafety {
		t.Fatalf("hostility relationship effects missing: before=%#v after=%#v", state.Relationship, next.Relationship)
	}
	if afterSafety.Satisfaction >= beforeSafety.Satisfaction || afterSafety.Urgency <= beforeSafety.Urgency {
		t.Fatalf("safety drive did not react: before=%#v after=%#v", beforeSafety, afterSafety)
	}
}

func TestLargeElapsedUsesBoundedAnalyticalRecovery(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	state.Emotions.Joy = 0.90
	state.Emotions.Sadness = 0.70
	next, log, err := Transition(state, nil, start.Add(10*365*24*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if log.RecoverySegments != 1 {
		t.Fatalf("large elapsed used %d recovery segments, want 1", log.RecoverySegments)
	}
	if next.Version != state.Version+1 {
		t.Fatalf("version = %d, want %d", next.Version, state.Version+1)
	}
	joyBaseline := profile.Dynamics.Dynamics[0].Baseline.Float64()
	if next.Emotions.Joy < joyBaseline || next.Emotions.Joy >= state.Emotions.Joy {
		t.Fatalf("joy did not recover toward baseline: before=%f after=%f baseline=%f", state.Emotions.Joy, next.Emotions.Joy, joyBaseline)
	}
	if err := next.Validate(); err != nil {
		t.Fatal(err)
	}
}

func loadTransitionFixture(t *testing.T) (AffectiveRuntimeProfile, AffectiveState, time.Time) {
	t.Helper()
	setAffectiveTestSecrets(t)
	root := filepath.Join("..", "..", "config")
	runtimeConfig, err := config.NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatalf("load repository config: %v", err)
	}
	profile, err := NewAffectiveRuntimeProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatalf("build runtime affective profile: %v", err)
	}
	start := time.Date(2026, time.July, 13, 18, 0, 0, 0, time.UTC)
	state, err := NewBaselineAffectiveStateFromProfiles(
		StateKey{IdentityID: "sonata", UserID: "user-1"},
		profile.Dynamics,
		profile.Initial,
		RelationshipState{Openness: 0.5, ConfidenceInUser: 0.5, PerceivedSafety: 0.5},
		start,
	)
	if err != nil {
		t.Fatalf("build baseline state: %v", err)
	}
	return profile, state, start
}

func driveByKind(t *testing.T, drives []DriveState, kind DriveKind) DriveState {
	t.Helper()
	for _, drive := range drives {
		if drive.Kind == kind {
			return drive
		}
	}
	t.Fatalf("drive %s not found", kind)
	return DriveState{}
}
