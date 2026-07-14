package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Lotargo/Sonata/internal/emotion"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAffectiveStateStore is the canonical v1 affective repository. State
// replacement and its version event are committed atomically.
type PostgresAffectiveStateStore struct {
	pool *pgxpool.Pool
}

func NewPostgresAffectiveStateStore(pool *pgxpool.Pool) (*PostgresAffectiveStateStore, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &PostgresAffectiveStateStore{pool: pool}, nil
}

func (store *PostgresAffectiveStateStore) Load(
	ctx context.Context,
	key emotion.StateKey,
) (emotion.AffectiveState, bool, error) {
	if store == nil || store.pool == nil {
		return emotion.AffectiveState{}, false, errors.New("Postgres affective store is not initialized")
	}
	if err := key.Validate(); err != nil {
		return emotion.AffectiveState{}, false, err
	}

	var (
		payload        []byte
		version        int64
		profileVersion string
		updatedAt      time.Time
	)
	err := store.pool.QueryRow(ctx, `
		SELECT state, version, profile_version, updated_at
		FROM sonata.affective_states
		WHERE identity_id = $1 AND owner_id = $2
	`, key.IdentityID, key.UserID).Scan(&payload, &version, &profileVersion, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return emotion.AffectiveState{}, false, nil
	}
	if err != nil {
		return emotion.AffectiveState{}, false, fmt.Errorf("load affective state: %w", err)
	}

	var state emotion.AffectiveState
	if err := json.Unmarshal(payload, &state); err != nil {
		return emotion.AffectiveState{}, false, fmt.Errorf("decode affective state: %w", err)
	}
	if state.Key != key || state.Version != version || state.ProfileVersion != profileVersion || !state.LastUpdatedAt.Equal(updatedAt) {
		return emotion.AffectiveState{}, false, errors.New("stored affective state envelope does not match indexed columns")
	}
	if err := state.Validate(); err != nil {
		return emotion.AffectiveState{}, false, fmt.Errorf("validate stored affective state: %w", err)
	}
	return state.Clone(), true, nil
}

func (store *PostgresAffectiveStateStore) CompareAndSwap(
	ctx context.Context,
	key emotion.StateKey,
	expectedVersion int64,
	next emotion.AffectiveState,
) error {
	if store == nil || store.pool == nil {
		return errors.New("Postgres affective store is not initialized")
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if expectedVersion < 0 {
		return errors.New("expected affective state version cannot be negative")
	}
	if next.Key != key {
		return errors.New("affective state key does not match store key")
	}
	if next.Version != expectedVersion+1 {
		return errors.New("affective state version must increment exactly once")
	}
	next.LastUpdatedAt = next.LastUpdatedAt.UTC().Truncate(time.Microsecond)
	if err := next.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("encode affective state: %w", err)
	}
	eventPayload, err := json.Marshal(struct {
		ProfileVersion string    `json:"profile_version"`
		UpdatedAt      time.Time `json:"updated_at"`
	}{ProfileVersion: next.ProfileVersion, UpdatedAt: next.LastUpdatedAt})
	if err != nil {
		return fmt.Errorf("encode affective state event: %w", err)
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin affective state transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO sonata.users (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING
	`, key.UserID); err != nil {
		return fmt.Errorf("ensure affective state owner: %w", err)
	}

	var affected int64
	if expectedVersion == 0 {
		tag, err := tx.Exec(ctx, `
			INSERT INTO sonata.affective_states (
				identity_id, owner_id, version, profile_version, state, updated_at
			)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
			ON CONFLICT (identity_id, owner_id) DO NOTHING
		`, key.IdentityID, key.UserID, next.Version, next.ProfileVersion, payload, next.LastUpdatedAt)
		if err != nil {
			return fmt.Errorf("insert affective state: %w", err)
		}
		affected = tag.RowsAffected()
	} else {
		tag, err := tx.Exec(ctx, `
			UPDATE sonata.affective_states
			SET version = $3,
				profile_version = $4,
				state = $5::jsonb,
				updated_at = $6
			WHERE identity_id = $1
				AND owner_id = $2
				AND version = $7
		`, key.IdentityID, key.UserID, next.Version, next.ProfileVersion, payload, next.LastUpdatedAt, expectedVersion)
		if err != nil {
			return fmt.Errorf("update affective state: %w", err)
		}
		affected = tag.RowsAffected()
	}
	if affected != 1 {
		return emotion.ErrVersionConflict
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sonata.affective_events (
			identity_id, owner_id, state_version, kind, payload, created_at
		)
		VALUES ($1, $2, $3, 'state_transition', $4::jsonb, $5)
	`, key.IdentityID, key.UserID, next.Version, eventPayload, next.LastUpdatedAt); err != nil {
		return fmt.Errorf("insert affective state event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit affective state transaction: %w", err)
	}
	return nil
}
