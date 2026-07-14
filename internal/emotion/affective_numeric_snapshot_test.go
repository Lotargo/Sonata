package emotion

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
	"time"
)

const affectiveNumericSnapshotSchemaV1 = "sonata-affective-numeric-snapshot-v1"

type affectiveNumericSnapshot struct {
	SchemaVersion  string                     `json:"schema_version"`
	Scenario       string                     `json:"scenario"`
	ProfileVersion string                     `json:"profile_version"`
	State          affectiveNumericState      `json:"state"`
	Transition     affectiveNumericTransition `json:"transition"`
}

type affectiveNumericState struct {
	Version       int64                        `json:"version"`
	LastUpdatedAt string                       `json:"last_updated_at"`
	Emotions      affectiveNumericEmotions     `json:"emotions"`
	Physiology    affectiveNumericPhysiology   `json:"physiology"`
	Relationship  affectiveNumericRelationship `json:"relationship"`
	Drives        []affectiveNumericDrive      `json:"drives"`
	ComplexStates []affectiveNumericComplex    `json:"complex_states"`
	Evidence      []affectiveNumericEvidence   `json:"evidence_accumulators"`
}

type affectiveNumericEmotions struct {
	Joy          float64 `json:"joy"`
	Trust        float64 `json:"trust"`
	Fear         float64 `json:"fear"`
	Surprise     float64 `json:"surprise"`
	Sadness      float64 `json:"sadness"`
	Disgust      float64 `json:"disgust"`
	Anger        float64 `json:"anger"`
	Anticipation float64 `json:"anticipation"`
}

type affectiveNumericPhysiology struct {
	Fatigue    float64 `json:"fatigue"`
	Arousal    float64 `json:"arousal"`
	Energy     float64 `json:"energy"`
	StressLoad float64 `json:"stress_load"`
	Stability  float64 `json:"stability"`
}

type affectiveNumericRelationship struct {
	Attachment       float64 `json:"attachment"`
	Openness         float64 `json:"openness"`
	Tension          float64 `json:"tension"`
	ConfidenceInUser float64 `json:"confidence_in_user"`
	PerceivedSafety  float64 `json:"perceived_safety"`
	UnresolvedHurt   float64 `json:"unresolved_hurt"`
}

type affectiveNumericDrive struct {
	Kind         string  `json:"kind"`
	Level        float64 `json:"level"`
	Satisfaction float64 `json:"satisfaction"`
	Urgency      float64 `json:"urgency"`
}

type affectiveNumericComplex struct {
	Kind         string  `json:"kind"`
	DefinitionID string  `json:"definition_id"`
	Activation   float64 `json:"activation"`
	ActiveSince  string  `json:"active_since"`
}

type affectiveNumericEvidence struct {
	Kind                   string  `json:"kind"`
	DefinitionID           string  `json:"definition_id"`
	PositiveArea           float64 `json:"positive_area"`
	ViolationArea          float64 `json:"violation_area"`
	ObservedForNanoseconds int64   `json:"observed_for_nanoseconds"`
	LastUpdatedAt          string  `json:"last_updated_at"`
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
		Kind:       StimulusUserWarmth,
		Source:     "numeric-golden-v1",
		Intensity:  1,
		Confidence: 1,
		Valence:    1,
		Arousal:    0.25,
		CreatedAt:  start,
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
		drives = append(drives, affectiveNumericDrive{
			Kind:         string(drive.Kind),
			Level:        snapshotFloat(drive.Level.Float64()),
			Satisfaction: snapshotFloat(drive.Satisfaction.Float64()),
			Urgency:      snapshotFloat(drive.Urgency.Float64()),
		})
	}
	complexStates := make([]affectiveNumericComplex, 0, len(state.ComplexStates))
	for _, active := range state.ComplexStates {
		complexStates = append(complexStates, affectiveNumericComplex{
			Kind:         string(active.Kind),
			DefinitionID: active.DefinitionID,
			Activation:   snapshotFloat(active.Activation.Float64()),
			ActiveSince:  snapshotTime(active.ActiveSince),
		})
	}
	evidence := make([]affectiveNumericEvidence, 0, len(state.Evidence))
	for _, accumulator := range state.Evidence {
		evidence = append(evidence, affectiveNumericEvidence{
			Kind:                   string(accumulator.Kind),
			DefinitionID:           accumulator.DefinitionID,
			PositiveArea:           snapshotFloat(accumulator.Evidence.PositiveArea.Float64()),
			ViolationArea:          snapshotFloat(accumulator.Evidence.ViolationArea.Float64()),
			ObservedForNanoseconds: accumulator.Evidence.ObservedFor.Nanoseconds(),
			LastUpdatedAt:          snapshotTime(accumulator.Evidence.LastUpdatedAt),
		})
	}
	return affectiveNumericSnapshot{
		SchemaVersion:  affectiveNumericSnapshotSchemaV1,
		Scenario:       scenario,
		ProfileVersion: state.ProfileVersion,
		State: affectiveNumericState{
			Version:       state.Version,
			LastUpdatedAt: snapshotTime(state.LastUpdatedAt),
			Emotions: affectiveNumericEmotions{
				Joy:          snapshotFloat(state.Emotions.Joy),
				Trust:        snapshotFloat(state.Emotions.Trust),
				Fear:         snapshotFloat(state.Emotions.Fear),
				Surprise:     snapshotFloat(state.Emotions.Surprise),
				Sadness:      snapshotFloat(state.Emotions.Sadness),
				Disgust:      snapshotFloat(state.Emotions.Disgust),
				Anger:        snapshotFloat(state.Emotions.Anger),
				Anticipation: snapshotFloat(state.Emotions.Anticipation),
			},
			Physiology: affectiveNumericPhysiology{
				Fatigue:    snapshotFloat(state.Physiology.Fatigue.Float64()),
				Arousal:    snapshotFloat(state.Physiology.Arousal.Float64()),
				Energy:     snapshotFloat(state.Physiology.Energy.Float64()),
				StressLoad: snapshotFloat(state.Physiology.StressLoad.Float64()),
				Stability:  snapshotFloat(state.Physiology.Stability.Float64()),
			},
			Relationship: affectiveNumericRelationship{
				Attachment:       snapshotFloat(state.Relationship.Attachment),
				Openness:         snapshotFloat(state.Relationship.Openness),
				Tension:          snapshotFloat(state.Relationship.Tension),
				ConfidenceInUser: snapshotFloat(state.Relationship.ConfidenceInUser),
				PerceivedSafety:  snapshotFloat(state.Relationship.PerceivedSafety),
				UnresolvedHurt:   snapshotFloat(state.Relationship.UnresolvedHurt),
			},
			Drives:        drives,
			ComplexStates: complexStates,
			Evidence:      evidence,
		},
		Transition: affectiveNumericTransition{
			RelationshipRule:    log.RelationshipRule,
			FromVersion:         log.FromVersion,
			ToVersion:           log.ToVersion,
			ElapsedNanoseconds:  log.Elapsed.Nanoseconds(),
			RecoverySegments:    log.RecoverySegments,
			AppliedStimuli:      log.AppliedStimuli,
			IntegrationSubsteps: log.IntegrationSubsteps,
			StimulusDefinitions: append([]string(nil), log.StimulusDefinitions...),
		},
	}
}

func snapshotFloat(value float64) float64 {
	return math.Round(value*1e9) / 1e9
}

func snapshotTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
