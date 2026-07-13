package emotion

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

func TestProfileFromConfigRequiresCompletePerUserProfile(t *testing.T) {
	value := testEmotionConfig()
	profile, err := NewProfileFromConfig("sonata", value)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Baseline.Trust != 0.45 || profile.DecayRates.Surprise != 0.20 || profile.MaxDelta != 0.20 {
		t.Fatalf("profile = %#v", profile)
	}
	delete(value.Baseline, "joy")
	if _, err := NewProfileFromConfig("sonata", value); err == nil {
		t.Fatal("missing baseline emotion was accepted")
	}
	value = testEmotionConfig()
	value.Relationship.Global = true
	if _, err := NewProfileFromConfig("sonata", value); err == nil {
		t.Fatal("global state was accepted")
	}
}

func TestEngineAppliesBoundedTransitionsAndOpposition(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	engine := testEngine(t, func() time.Time { return now })
	key := engine.Key("user-1")
	report, err := engine.ApplyStimuli(context.Background(), key, []Stimulus{{
		Kind: StimulusUserHostility, Source: "test", Intensity: 1, Confidence: 1, Valence: -1, Arousal: 1, CreatedAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if report.StateVersion != 1 || report.Status != StatusHealthy {
		t.Fatalf("report = %#v", report)
	}
	state, exists, err := engine.store.Load(context.Background(), key)
	if err != nil || !exists {
		t.Fatalf("load state: exists=%t err=%v", exists, err)
	}
	if state.Emotions.Anger <= 0.05 || state.Emotions.Fear >= 0.16 {
		t.Fatalf("opposition was not applied: %#v", state.Emotions)
	}
	if state.Emotions.Anger > 1 || state.Relationship.Tension > 1 || state.Stability < 0 {
		t.Fatalf("state escaped bounds: %#v", state)
	}
	if len(report.Text) > 240 || strings.Contains(report.Text, "test") {
		t.Fatalf("report is not compact or leaked source: %q", report.Text)
	}
}

func TestLazyDecayMovesStateTowardBaseline(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	engine := testEngine(t, func() time.Time { return now })
	key := engine.Key("user-1")
	_, err := engine.ApplyStimuli(context.Background(), key, []Stimulus{{
		Kind: StimulusUserSuccess, Source: "test", Intensity: 1, Confidence: 1, Valence: 1, Arousal: 0.5, CreatedAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	before, _, _ := engine.store.Load(context.Background(), key)
	now = now.Add(24 * time.Hour)
	report, err := engine.GetReport(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	after, _, _ := engine.store.Load(context.Background(), key)
	baseline := engine.profile.Baseline.Joy
	if !(after.Emotions.Joy < before.Emotions.Joy && after.Emotions.Joy > baseline) {
		t.Fatalf("joy did not decay toward baseline: before=%f after=%f baseline=%f", before.Emotions.Joy, after.Emotions.Joy, baseline)
	}
	if report.StateVersion != before.Version+1 {
		t.Fatalf("decay version = %d, want %d", report.StateVersion, before.Version+1)
	}
}

func TestStateIsIsolatedByIdentityAndUser(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	engine := testEngine(t, func() time.Time { return now })
	first := engine.Key("first")
	second := engine.Key("second")
	_, err := engine.ApplyStimuli(context.Background(), first, []Stimulus{{
		Kind: StimulusPromiseKept, Source: "test", Intensity: 1, Confidence: 1, Valence: 1, CreatedAt: now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	firstReport, err := engine.GetReport(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	secondReport, err := engine.GetReport(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if firstReport.StateVersion == secondReport.StateVersion && firstReport.Relationship.ConfidenceInUser == secondReport.Relationship.ConfidenceInUser {
		t.Fatalf("user states were mixed: first=%#v second=%#v", firstReport, secondReport)
	}
	foreign := StateKey{IdentityID: "other", UserID: "first"}
	if _, err := engine.GetReport(context.Background(), foreign); err == nil {
		t.Fatal("foreign identity was accepted")
	}
}

func TestConcurrentUpdatesUseOptimisticVersions(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	engine := testEngine(t, func() time.Time { return now })
	key := engine.Key("user-1")
	const updates = 32
	var wait sync.WaitGroup
	errorsChannel := make(chan error, updates)
	for index := 0; index < updates; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := engine.ApplyStimuli(context.Background(), key, []Stimulus{{
				Kind: StimulusToolSuccess, Source: "test", Intensity: 0.1, Confidence: 1, Valence: 0.5, CreatedAt: now,
			}})
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, exists, err := engine.store.Load(context.Background(), key)
	if err != nil || !exists {
		t.Fatalf("load state: exists=%t err=%v", exists, err)
	}
	if state.Version != updates {
		t.Fatalf("state version = %d, want %d", state.Version, updates)
	}
}

func TestExtractorIsDeterministicAndDoesNotRetainMessage(t *testing.T) {
	extractor := Extractor{}
	at := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	message := "СПАСИБО!!! Я тебе доверяю"
	first := extractor.ExtractUserMessage(message, at)
	second := extractor.ExtractUserMessage(message, at)
	if len(first) != 2 || len(second) != len(first) {
		t.Fatalf("stimuli = %#v", first)
	}
	for index := range first {
		if !reflect.DeepEqual(first[index], second[index]) {
			t.Fatalf("extractor is not deterministic: %#v %#v", first, second)
		}
		if len(first[index].Metadata) != 0 || strings.Contains(first[index].Source, message) || strings.Contains(first[index].Target, message) {
			t.Fatalf("raw message was retained: %#v", first[index])
		}
	}
}

func TestIdenticalStimulusSequenceIsDeterministic(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	firstEngine := testEngine(t, func() time.Time { return now })
	secondEngine := testEngine(t, func() time.Time { return now })
	stimuli := []Stimulus{
		{Kind: StimulusUserHostility, Source: "test", Intensity: 0.9, Confidence: 0.8, Valence: -1, Arousal: 1, CreatedAt: now},
		{Kind: StimulusUserApology, Source: "test", Intensity: 0.7, Confidence: 0.9, Valence: 0.5, Arousal: 0.2, CreatedAt: now},
		{Kind: StimulusPromiseKept, Source: "test", Intensity: 0.8, Confidence: 1, Valence: 1, Arousal: 0.4, CreatedAt: now},
	}

	firstReport, err := firstEngine.ApplyStimuli(context.Background(), firstEngine.Key("user-1"), stimuli)
	if err != nil {
		t.Fatal(err)
	}
	secondReport, err := secondEngine.ApplyStimuli(context.Background(), secondEngine.Key("user-1"), stimuli)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstReport, secondReport) {
		t.Fatalf("identical stimuli produced different reports:\nfirst:  %#v\nsecond: %#v", firstReport, secondReport)
	}
}

func TestGetReportOrBaselineDegradesOnStoreFailure(t *testing.T) {
	profile, err := NewProfileFromConfig("sonata", testEmotionConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	engine, err := NewEngine(profile, failingStore{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	report := engine.GetReportOrBaseline(context.Background(), engine.Key("user-1"))
	if report.Status != StatusDegraded || report.StateVersion != 0 || report.DominantEmotions[0].Name != Trust {
		t.Fatalf("degraded report = %#v", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

type failingStore struct{}

func (failingStore) Load(context.Context, StateKey) (State, bool, error) {
	return State{}, false, errors.New("store unavailable")
}

func (failingStore) CompareAndSwap(context.Context, StateKey, int64, State) error {
	return errors.New("store unavailable")
}

func testEngine(t *testing.T, clock Clock) *Engine {
	t.Helper()
	profile, err := NewProfileFromConfig("sonata", testEmotionConfig())
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(profile, NewMemoryStore(), clock)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func testEmotionConfig() config.EmotionConfig {
	return config.EmotionConfig{
		Baseline: map[string]float64{
			"joy": 0.35, "trust": 0.45, "fear": 0.10, "surprise": 0.15,
			"sadness": 0.10, "disgust": 0.05, "anger": 0.05, "anticipation": 0.30,
		},
		DecayRates: map[string]float64{
			"joy": 0.08, "trust": 0.02, "fear": 0.10, "surprise": 0.20,
			"sadness": 0.04, "disgust": 0.06, "anger": 0.08, "anticipation": 0.06,
		},
		MaxDelta:     0.20,
		Relationship: config.RelationshipConfig{PerUser: true, Global: false},
	}
}
