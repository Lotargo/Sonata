package emotion

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAffectiveRuntimePersistsVersionedOwnerScopedState(t *testing.T) {
	profile, _, start := loadTransitionFixture(t)
	now := start
	store := NewMemoryAffectiveStateStore()
	runtime, err := NewAffectiveRuntime("sonata", profile, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	first, err := runtime.ProcessUserMessage(context.Background(), "user-1", "СПАСИБО!!!")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != StatusHealthy || first.StateVersion != 1 || first.ProfileVersion != profile.Dynamics.Version {
		t.Fatalf("first report = %#v", first)
	}
	if strings.Contains(strings.ToLower(first.Text), "спасибо") {
		t.Fatalf("report retained raw user text: %q", first.Text)
	}
	firstState, exists, err := store.Load(context.Background(), runtime.Key("user-1"))
	if err != nil || !exists {
		t.Fatalf("load first state: exists=%t err=%v", exists, err)
	}
	if firstState.Version != 1 || firstState.Emotions.Trust <= profile.Dynamics.Dynamics[1].Baseline.Float64() {
		t.Fatalf("warmth transition was not persisted: %#v", firstState)
	}

	now = now.Add(time.Hour)
	second, err := runtime.ProcessUserMessage(context.Background(), "user-1", "ненавижу")
	if err != nil {
		t.Fatal(err)
	}
	if second.StateVersion != 2 {
		t.Fatalf("second state version = %d, want 2", second.StateVersion)
	}

	other, err := runtime.ProcessUserMessage(context.Background(), "user-2", "спасибо")
	if err != nil {
		t.Fatal(err)
	}
	if other.StateVersion != 1 {
		t.Fatalf("other user state version = %d, want 1", other.StateVersion)
	}
	otherState, exists, err := store.Load(context.Background(), runtime.Key("user-2"))
	if err != nil || !exists {
		t.Fatalf("load other state: exists=%t err=%v", exists, err)
	}
	if otherState.Key.UserID != "user-2" || otherState.Version != 1 {
		t.Fatalf("owner state mixed: %#v", otherState)
	}
}

func TestAffectiveRuntimeConcurrentUpdatesUseCAS(t *testing.T) {
	profile, _, start := loadTransitionFixture(t)
	store := NewMemoryAffectiveStateStore()
	runtime, err := NewAffectiveRuntime("sonata", profile, store, func() time.Time { return start })
	if err != nil {
		t.Fatal(err)
	}

	const updates = 16
	var wait sync.WaitGroup
	errorsChannel := make(chan error, updates)
	for index := 0; index < updates; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := runtime.ProcessUserMessage(context.Background(), "user-1", "спасибо")
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
	state, exists, err := store.Load(context.Background(), runtime.Key("user-1"))
	if err != nil || !exists {
		t.Fatalf("load state: exists=%t err=%v", exists, err)
	}
	if state.Version != updates {
		t.Fatalf("state version = %d, want %d", state.Version, updates)
	}
}

func TestAffectiveRuntimeBuildsDegradedBaselineWithoutStoreWrite(t *testing.T) {
	profile, _, start := loadTransitionFixture(t)
	store := failingAffectiveStateStore{}
	runtime, err := NewAffectiveRuntime("sonata", profile, store, func() time.Time { return start })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ProcessUserMessage(context.Background(), "user-1", "спасибо"); err == nil {
		t.Fatal("store failure was hidden by canonical transition")
	}
	report, err := runtime.DegradedReport("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != StatusDegraded || report.StateVersion != 0 || !strings.Contains(report.Text, "status=DEGRADED") {
		t.Fatalf("degraded report = %#v", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

type failingAffectiveStateStore struct{}

func (failingAffectiveStateStore) Load(context.Context, StateKey) (AffectiveState, bool, error) {
	return AffectiveState{}, false, errors.New("store unavailable")
}

func (failingAffectiveStateStore) CompareAndSwap(context.Context, StateKey, int64, AffectiveState) error {
	return errors.New("store unavailable")
}
