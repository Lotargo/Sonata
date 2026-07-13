package emotion

import (
	"context"
	"errors"
	"sync"
)

var ErrVersionConflict = errors.New("emotion state version conflict")

type Store interface {
	Load(context.Context, StateKey) (State, bool, error)
	CompareAndSwap(context.Context, StateKey, int64, State) error
}

type MemoryStore struct {
	mu     sync.RWMutex
	states map[StateKey]State
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{states: make(map[StateKey]State)}
}

func (store *MemoryStore) Load(ctx context.Context, key StateKey) (State, bool, error) {
	if err := ctx.Err(); err != nil {
		return State{}, false, err
	}
	if err := key.Validate(); err != nil {
		return State{}, false, err
	}
	store.mu.RLock()
	state, exists := store.states[key]
	store.mu.RUnlock()
	return state, exists, nil
}

func (store *MemoryStore) CompareAndSwap(ctx context.Context, key StateKey, expectedVersion int64, next State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if next.Key != key {
		return errors.New("emotion state key does not match store key")
	}
	if next.Version != expectedVersion+1 {
		return errors.New("emotion state version must increment exactly once")
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
	store.states[key] = next
	return nil
}
