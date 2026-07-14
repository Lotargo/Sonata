package emotion

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

const affectiveNumericSnapshotSchemaV1 = "sonata-affective-numeric-snapshot-v1"

type affectiveNumericSnapshot struct {
	SchemaVersion  string                    `json:"schema_version"`
	Scenario       string                    `json:"scenario"`
	ProfileVersion string                    `json:"profile_version"`
	State          affectiveNumericState     `json:"state"`
	Transition     affectiveNumericTransition `json:"transition"`
}

type affectiveNumericState struct {
	Version        int64                       `json:"version"`
	LastUpdatedAt  string                      `json:"last_updated_at"`
	Emotions       affectiveNumericEmotions    `json:"emotions"`
	Physiology     affectiveNumericPhysiology  `json:"physiology"`
	Relationship   affectiveNumericRelationship `json:"relationship"`
	Drives         []affectiveNumericDrive     `json:"drives"`
	ComplexStates  int                         `json:"complex_states"`
	Evidence       int                         `json:"evidence_accumulators"`
}

type affectiveNumericEmotions struct {
	Joy, Trust, Fear, Surprise, Sadness, Disgust, Anger, Anticipation float64
}

type affectiveNumericPhysiology struct {
	Fatigue, Arousal, Energy, StressLoad, Stability float64
}

type affectiveNumericRelationship struct {
	Attachment, Openness, Tension, ConfidenceInUser, PerceivedSafety, UnresolvedHurt float64
}

type affectiveNumericDrive struct {
	Kind         string  `json:"kind"`
	Level        float64 `json:"level"`
	Satisfaction float64 `json:"satisfaction"`
	Urgency      float64 `json:"urgency"`
}

type affectiveNumericTransition struct {
	RelationshipRule    string   `json:"relationship_rule"`
	FromVersion         int64    `json:"from_version"`
	ToVersion           int64    `json:"to_version"`
	ElapsedNanoseconds  int64    `json:"elapsed_nanoseconds"`
	RecoverySegments    int      `json:"recovery_segments"`
	AppliedStimuli      int      `json:"applied_stimuli"`
	IntegrationSubsteps int      `json:"integration_substeps"`
	StimulusDefinitions []string `json:"stimulus_definitions"`
}

func TestGoldenV1NumericSnapshotWarmthAtBaseline(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	assertGoldenProfileVersion(t, profile)

	next, log, err := Transition(initial, []Stimulus{{
		Kind: StimulusUserWarmth, Source: "numeric-golden-v1", Intensity: 1, Confidence: 1,
		Valence: 1, Arousal: 0.25, CreatedAt: start,
	}}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	actual := buildAffectiveNumericSnapshot("warmth_at_baseline", next, log)
	data, err := os.ReadFile("testdata/affective_numeric_v1_warmth.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected affectiveNumericSnapshot
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		actualJSON, _ := json.MarshalIndent(actual, "", "  ")
		expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Fatalf("numeric snapshot drifted\nexpected:\n%s\nactual:\n%s", expectedJSON, actualJSON)
	}
}

func buildAffectiveNumericSnapshot(scenario string, state AffectiveState, log TransitionLog) affectiveNumericSnapshot {
	drives := make([]affectiveNumericDrive, 0, len(state.Drives))
	for _, drive := range state.Drives {
		drives = append(drives, affectiveNumericDrive{string(drive.Kind), snapshotFloat(drive.Level.Float64()), snapshotFloat(drive.Satisfaction.Float64()), snapshotFloat(drive.Urgency.Float64())})
	}
	return affectiveNumericSnapshot{
		SchemaVersion: affectiveNumericSnapshotSchemaV1, Scenario: scenario, ProfileVersion: state.ProfileVersion,
		State: affectiveNumericState{
			Version: state.Version, LastUpdatedAt: state.LastUpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Emotions: affectiveNumericEmotions{snapshotFloat(state.Emotions.Joy), snapshotFloat(state.Emotions.Trust), snapshotFloat(state.Emotions.Fear), snapshotFloat(state.Emotions.Surprise), snapshotFloat(state.Emotions.Sadness), snapshotFloat(state.Emotions.Disgust), snapshotFloat(state.Emotions.Anger), snapshotFloat(state.Emotions.Anticipation)},
			Physiology: affectiveNumericPhysiology{snapshotFloat(state.Physiology.Fatigue.Float64()), snapshotFloat(state.Physiology.Arousal.Float64()), snapshotFloat(state.Physiology.Energy.Float64()), snapshotFloat(state.Physiology.StressLoad.Float64()), snapshotFloat(state.Physiology.Stability.Float64())},
			Relationship: affectiveNumericRelationship{snapshotFloat(state.Relationship.Attachment), snapshotFloat(state.Relationship.Openness), snapshotFloat(state.Relationship.Tension), snapshotFloat(state.Relationship.ConfidenceInUser), snapshotFloat(state.Relationship.PerceivedSafety), snapshotFloat(state.Relationship.UnresolvedHurt)},
			Drives: drives, ComplexStates: len(state.ComplexStates), Evidence: len(state.Evidence),
		},
		Transition: affectiveNumericTransition{log.RelationshipRule, log.FromVersion, log.ToVersion, log.Elapsed.Nanoseconds(), log.RecoverySegments, log.AppliedStimuli, log.IntegrationSubsteps, append([]string(nil), log.StimulusDefinitions...)},
	}
}

func snapshotFloat(value float64) float64 { return math.Round(value*1e9) / 1e9 }
