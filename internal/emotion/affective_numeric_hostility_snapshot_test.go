package emotion

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestGoldenV1NumericSnapshotHostilityAtBaseline(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	assertGoldenProfileVersion(t, profile)

	next, log, err := Transition(initial, []Stimulus{{
		Kind:       StimulusUserHostility,
		Source:     "numeric-golden-v1",
		Intensity:  1,
		Confidence: 1,
		Valence:    -1,
		Arousal:    1,
		CreatedAt:  start,
	}}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	actual := buildAffectiveNumericSnapshot("hostility_at_baseline", next, log)
	data, err := os.ReadFile("testdata/affective_numeric_v1_hostility.json")
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
