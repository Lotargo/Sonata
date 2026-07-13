package emotion

import (
	"testing"
	"time"
)

const goldenAffectiveProfileV1 = "sonata-affective-v1.0.0"

func TestGoldenV1WarmthAndTrustAccumulate(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	assertGoldenProfileVersion(t, profile)

	warmed, _, err := Transition(initial, []Stimulus{{
		Kind:       StimulusUserWarmth,
		Source:     "golden-v1",
		Intensity:  1,
		Confidence: 1,
		Valence:    1,
		Arousal:    0.25,
		CreatedAt:  start,
	}}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	trusted, _, err := Transition(warmed, []Stimulus{{
		Kind:       StimulusUserTrust,
		Source:     "golden-v1",
		Intensity:  1,
		Confidence: 1,
		Valence:    1,
		Arousal:    0.20,
		CreatedAt:  start,
	}}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	if warmed.Emotions.Joy <= initial.Emotions.Joy || warmed.Emotions.Trust <= initial.Emotions.Trust {
		t.Fatalf("warmth did not raise joy and trust: initial=%#v warmed=%#v", initial.Emotions, warmed.Emotions)
	}
	if trusted.Emotions.Joy <= warmed.Emotions.Joy || trusted.Emotions.Trust <= warmed.Emotions.Trust {
		t.Fatalf("trust did not accumulate after warmth: warmed=%#v trusted=%#v", warmed.Emotions, trusted.Emotions)
	}
	if trusted.Relationship.Attachment <= initial.Relationship.Attachment ||
		trusted.Relationship.Openness <= initial.Relationship.Openness ||
		trusted.Relationship.PerceivedSafety <= initial.Relationship.PerceivedSafety ||
		trusted.Relationship.ConfidenceInUser <= initial.Relationship.ConfidenceInUser {
		t.Fatalf("positive relationship trajectory did not accumulate: initial=%#v trusted=%#v", initial.Relationship, trusted.Relationship)
	}
}

func TestGoldenV1HostilityThenApologyRepairsState(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	assertGoldenProfileVersion(t, profile)

	hostile, _, err := Transition(initial, []Stimulus{{
		Kind:       StimulusUserHostility,
		Source:     "golden-v1",
		Intensity:  1,
		Confidence: 1,
		Valence:    -1,
		Arousal:    1,
		CreatedAt:  start,
	}}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	repaired, _, err := Transition(hostile, []Stimulus{{
		Kind:       StimulusUserApology,
		Source:     "golden-v1",
		Intensity:  1,
		Confidence: 1,
		Valence:    0.8,
		Arousal:    -0.25,
		CreatedAt:  start,
	}}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	if hostile.Emotions.Anger <= initial.Emotions.Anger || hostile.Emotions.Trust >= initial.Emotions.Trust {
		t.Fatalf("hostility trajectory was not applied: initial=%#v hostile=%#v", initial.Emotions, hostile.Emotions)
	}
	if repaired.Emotions.Anger >= hostile.Emotions.Anger || repaired.Emotions.Trust <= hostile.Emotions.Trust {
		t.Fatalf("apology did not repair emotional state: hostile=%#v repaired=%#v", hostile.Emotions, repaired.Emotions)
	}
	if repaired.Relationship.Tension >= hostile.Relationship.Tension ||
		repaired.Relationship.UnresolvedHurt >= hostile.Relationship.UnresolvedHurt ||
		repaired.Relationship.Openness <= hostile.Relationship.Openness {
		t.Fatalf("apology did not repair relationship state: hostile=%#v repaired=%#v", hostile.Relationship, repaired.Relationship)
	}
	if repaired.Physiology.StressLoad >= hostile.Physiology.StressLoad || repaired.Physiology.Stability <= hostile.Physiology.Stability {
		t.Fatalf("apology did not reduce physiological stress: hostile=%#v repaired=%#v", hostile.Physiology, repaired.Physiology)
	}
}

func TestGoldenV1RepeatedToolFailureRaisesFatigueThenQuietRecovers(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	assertGoldenProfileVersion(t, profile)

	failures := make([]Stimulus, 3)
	for index := range failures {
		failures[index] = Stimulus{
			Kind:       StimulusToolFailure,
			Source:     "golden-v1",
			Intensity:  1,
			Confidence: 1,
			Valence:    -0.6,
			Arousal:    0.6,
			CreatedAt:  start,
		}
	}

	fatigued, _, err := Transition(initial, failures, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	if fatigued.Physiology.Fatigue <= initial.Physiology.Fatigue || fatigued.Physiology.StressLoad <= initial.Physiology.StressLoad {
		t.Fatalf("repeated failures did not raise fatigue and stress: initial=%#v fatigued=%#v", initial.Physiology, fatigued.Physiology)
	}

	recovered, log, err := Transition(fatigued, nil, start.Add(24*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if log.IntegrationSubsteps <= 0 || log.IntegrationSubsteps > profile.Dynamics.MaxSubsteps {
		t.Fatalf("quiet recovery used invalid substep count: %d", log.IntegrationSubsteps)
	}
	if recovered.Physiology.Fatigue >= fatigued.Physiology.Fatigue || recovered.Physiology.StressLoad >= fatigued.Physiology.StressLoad {
		t.Fatalf("quiet interval did not recover physiology: fatigued=%#v recovered=%#v", fatigued.Physiology, recovered.Physiology)
	}
}

func TestGoldenV1FearAndAngerRemainMixedAndBounded(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	assertGoldenProfileVersion(t, profile)

	next, _, err := Transition(initial, []Stimulus{
		{
			Kind:       StimulusUserDistress,
			Source:     "golden-v1",
			Intensity:  1,
			Confidence: 1,
			Valence:    -0.8,
			Arousal:    0.8,
			CreatedAt:  start,
		},
		{
			Kind:       StimulusUserHostility,
			Source:     "golden-v1",
			Intensity:  1,
			Confidence: 1,
			Valence:    -1,
			Arousal:    1,
			CreatedAt:  start,
		},
	}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	if next.Emotions.Fear <= initial.Emotions.Fear || next.Emotions.Anger <= initial.Emotions.Anger {
		t.Fatalf("mixed fear and anger state was collapsed: initial=%#v next=%#v", initial.Emotions, next.Emotions)
	}
	if next.Emotions.Fear > 1 || next.Emotions.Anger > 1 {
		t.Fatalf("mixed state escaped bounds: %#v", next.Emotions)
	}
	if err := next.Validate(); err != nil {
		t.Fatalf("mixed state is invalid: %v", err)
	}
}

func assertGoldenProfileVersion(t *testing.T, profile AffectiveRuntimeProfile) {
	t.Helper()
	if profile.Dynamics.Version != goldenAffectiveProfileV1 ||
		profile.Initial.ProfileVersion != goldenAffectiveProfileV1 ||
		profile.Stimuli.ProfileVersion != goldenAffectiveProfileV1 {
		t.Fatalf("golden v1 tests require profile %q, got dynamics=%q initial=%q stimuli=%q",
			goldenAffectiveProfileV1,
			profile.Dynamics.Version,
			profile.Initial.ProfileVersion,
			profile.Stimuli.ProfileVersion,
		)
	}
}
