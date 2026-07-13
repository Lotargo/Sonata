package emotion

import (
	"reflect"
	"testing"
	"time"
)

func TestTransitionReplayProducesIdenticalTrajectory(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)
	timeline := []Stimulus{
		{
			Kind:       StimulusUserWarmth,
			Source:     "replay-test",
			Intensity:  0.75,
			Confidence: 1,
			Valence:    0.8,
			Arousal:    0.25,
			CreatedAt:  start.Add(20 * time.Minute),
		},
		{
			Kind:       StimulusUserHostility,
			Source:     "replay-test",
			Intensity:  0.55,
			Confidence: 0.9,
			Valence:    -1,
			Arousal:    0.8,
			CreatedAt:  start.Add(2 * time.Hour),
		},
		{
			Kind:       StimulusUserSuccess,
			Source:     "replay-test",
			Intensity:  0.9,
			Confidence: 1,
			Valence:    1,
			Arousal:    0.5,
			CreatedAt:  start.Add(5 * time.Hour),
		},
	}

	run := func() ([]AffectiveState, []TransitionLog) {
		current := initial.Clone()
		states := make([]AffectiveState, 0, len(timeline))
		logs := make([]TransitionLog, 0, len(timeline))
		for _, stimulus := range timeline {
			next, log, err := Transition(current, []Stimulus{stimulus}, stimulus.CreatedAt, profile)
			if err != nil {
				t.Fatalf("replay transition at %s: %v", stimulus.CreatedAt, err)
			}
			states = append(states, next)
			logs = append(logs, log)
			current = next
		}
		return states, logs
	}

	firstStates, firstLogs := run()
	secondStates, secondLogs := run()
	if !reflect.DeepEqual(firstStates, secondStates) {
		t.Fatalf("replayed states differ:\nfirst=%#v\nsecond=%#v", firstStates, secondStates)
	}
	if !reflect.DeepEqual(firstLogs, secondLogs) {
		t.Fatalf("replayed transition logs differ:\nfirst=%#v\nsecond=%#v", firstLogs, secondLogs)
	}
}

func TestTransitionMaintainsInvariantsAcrossDeterministicCorpus(t *testing.T) {
	profile, initial, start := loadTransitionFixture(t)

	for seed := 0; seed < 64; seed++ {
		data := deterministicStimulusCorpus(uint32(seed+1), 24)
		stimuli := stimuliFromBytes(data, start)
		now := start.Add(time.Duration(len(stimuli)+1) * time.Minute)

		next, _, err := Transition(initial, stimuli, now, profile)
		if err != nil {
			t.Fatalf("seed %d transition: %v", seed, err)
		}
		if err := next.Validate(); err != nil {
			t.Fatalf("seed %d produced invalid state: %v", seed, err)
		}
		if next.Key != initial.Key {
			t.Fatalf("seed %d changed owner key: before=%#v after=%#v", seed, initial.Key, next.Key)
		}
		if next.Version != initial.Version+1 {
			t.Fatalf("seed %d version=%d, want %d", seed, next.Version, initial.Version+1)
		}
		if next.LastUpdatedAt != now.UTC() {
			t.Fatalf("seed %d timestamp=%s, want %s", seed, next.LastUpdatedAt, now.UTC())
		}
	}
}

func TestLongHorizonTransitionCapsIntegrationSubsteps(t *testing.T) {
	profile, state, start := loadTransitionFixture(t)
	state.Emotions.Joy = 0.95
	state.Emotions.Sadness = 0.85
	state.Physiology.Fatigue = 1
	state.Physiology.StressLoad = 1

	next, log, err := Transition(state, nil, start.Add(100*365*24*time.Hour), profile)
	if err != nil {
		t.Fatal(err)
	}
	if log.IntegrationSubsteps <= 0 {
		t.Fatal("long-horizon transition did not execute integration substeps")
	}
	if log.IntegrationSubsteps > profile.Dynamics.MaxSubsteps {
		t.Fatalf("integration substeps=%d exceed configured maximum=%d", log.IntegrationSubsteps, profile.Dynamics.MaxSubsteps)
	}
	if err := next.Validate(); err != nil {
		t.Fatalf("long-horizon transition produced invalid state: %v", err)
	}
}

func FuzzTransitionMaintainsDeterministicBounds(f *testing.F) {
	f.Add([]byte{0x00, 0x2f, 0x71, 0xa4, 0xff})
	f.Add([]byte{0x11, 0x11, 0x11})
	f.Add([]byte{0xfe, 0x80, 0x40, 0x20, 0x10, 0x08})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64 {
			data = data[:64]
		}
		profile, initial, start := loadTransitionFixture(t)
		stimuli := stimuliFromBytes(data, start)
		now := start.Add(time.Duration(len(stimuli)+1) * time.Minute)

		first, firstLog, err := Transition(initial, stimuli, now, profile)
		if err != nil {
			t.Fatalf("transition: %v", err)
		}
		second, secondLog, err := Transition(initial, stimuli, now, profile)
		if err != nil {
			t.Fatalf("replayed transition: %v", err)
		}
		if err := first.Validate(); err != nil {
			t.Fatalf("invalid state: %v", err)
		}
		if first.Key != initial.Key {
			t.Fatalf("owner key changed: before=%#v after=%#v", initial.Key, first.Key)
		}
		if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstLog, secondLog) {
			t.Fatalf("same input produced different transition result")
		}
	})
}

func deterministicStimulusCorpus(seed uint32, size int) []byte {
	result := make([]byte, size)
	value := seed
	for index := range result {
		value ^= value << 13
		value ^= value >> 17
		value ^= value << 5
		result[index] = byte(value)
	}
	return result
}

func stimuliFromBytes(data []byte, start time.Time) []Stimulus {
	stimuli := make([]Stimulus, 0, len(data))
	for index, value := range data {
		kind, valence, arousal := stimulusShape(value)
		stimuli = append(stimuli, Stimulus{
			Kind:       kind,
			Source:     "trajectory-test",
			Intensity:  float64(value&0x0f) / 15,
			Confidence: 0.5 + float64((value>>4)&0x0f)/30,
			Valence:    valence,
			Arousal:    arousal,
			CreatedAt:  start.Add(time.Duration(index+1) * time.Minute),
		})
	}
	return stimuli
}

func stimulusShape(value byte) (StimulusKind, float64, float64) {
	switch value % 3 {
	case 0:
		return StimulusUserWarmth, 0.8, 0.25
	case 1:
		return StimulusUserHostility, -1, 0.9
	default:
		return StimulusUserSuccess, 1, 0.5
	}
}
