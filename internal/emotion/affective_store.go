package emotion

import (
	"context"
	"errors"
	"sync"
)

// AffectiveStateStore preserves optimistic version semantics for the v1
// affective state. The in-memory implementation is transitional until the
// canonical Neon repository is introduced in stage 08.
type AffectiveStateStore interface {
	Load(context.Context, StateKey) (AffectiveState, bool, error)
	CompareAndSwap(context.Context, StateKey, int64, AffectiveState) error
}

type MemoryAffectiveStateStore struct {
	mu     sync.RWMutex
	states map[StateKey]AffectiveState
}

func NewMemoryAffectiveStateStore() *MemoryAffectiveStateStore {
	return &MemoryAffectiveStateStore{states: make(map[StateKey]AffectiveState)}
}

func (store *MemoryAffectiveStateStore) Load(ctx context.Context, key StateKey) (AffectiveState, bool, error) {
	if err := ctx.Err(); err != nil {
		return AffectiveState{}, false, err
	}
	if err := key.Validate(); err != nil {
		return AffectiveState{}, false, err
	}
	store.mu.RLock()
	state, exists := store.states[key]
	store.mu.RUnlock()
	if !exists {
		return AffectiveState{}, false, nil
	}
	return state.Clone(), true, nil
}

func (store *MemoryAffectiveStateStore) CompareAndSwap(ctx context.Context, key StateKey, expectedVersion int64, next AffectiveState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if next.Key != key {
		return errors.New("affective state key does not match store key")
	}
	if next.Version != expectedVersion+1 {
		return errors.New("affective state version must increment exactly once")
	}
	if err := next.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	current, exists := store.states[key]
	if exists {
		if current.Version != expectedVersion {
			return ErrVersionConflict
		}
	} else if expectedVersion != 0 {
		return ErrVersionConflict
	}
	store.states[key] = next.Clone()
	return nil
}
