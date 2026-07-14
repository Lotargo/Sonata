package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/database/dbgen"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRepository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewOutboxRepository(pool *pgxpool.Pool) (*OutboxRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &OutboxRepository{pool: pool, queries: dbgen.New(pool)}, nil
}

type InsertOutboxEventInput struct {
	OwnerID       *string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
	Status        string // usually "pending"
	Attempts      int32
	AvailableAt   time.Time
	CreatedAt     time.Time
}

func (r *OutboxRepository) Insert(ctx context.Context, input InsertOutboxEventInput) (dbgen.OutboxEvent, error) {
	if r == nil || r.queries == nil {
		return dbgen.OutboxEvent{}, errors.New("outbox repository is not initialized")
	}
	if strings.TrimSpace(input.AggregateType) == "" || strings.TrimSpace(input.AggregateID) == "" || strings.TrimSpace(input.EventType) == "" {
		return dbgen.OutboxEvent{}, errors.New("aggregate type, aggregate ID, and event type are required")
	}
	createdAt, err := requiredDatabaseTime(input.CreatedAt, "outbox event creation time")
	if err != nil {
		return dbgen.OutboxEvent{}, err
	}
	availableAt, err := requiredDatabaseTime(input.AvailableAt, "outbox event availability time")
	if err != nil {
		return dbgen.OutboxEvent{}, err
	}
	payload := normalizedJSON(input.Payload)

	return r.queries.InsertOutboxEvent(ctx, dbgen.InsertOutboxEventParams{
		OwnerID:       input.OwnerID,
		AggregateType: input.AggregateType,
		AggregateID:   input.AggregateID,
		EventType:     input.EventType,
		Payload:       payload,
		Status:        input.Status,
		Attempts:      input.Attempts,
		AvailableAt:   availableAt,
		CreatedAt:     createdAt,
	})
}

func (r *OutboxRepository) LockPending(ctx context.Context, lockedAt time.Time, now time.Time, limit int32) ([]dbgen.OutboxEvent, error) {
	if r == nil || r.queries == nil {
		return nil, errors.New("outbox repository is not initialized")
	}
	lockedAtTime, err := requiredDatabaseTime(lockedAt, "lock time")
	if err != nil {
		return nil, err
	}
	nowTime, err := requiredDatabaseTime(now, "now time")
	if err != nil {
		return nil, err
	}
	return r.queries.LockPendingOutboxEvents(ctx, dbgen.LockPendingOutboxEventsParams{
		LockedAt:   &lockedAtTime,
		Now:        nowTime,
		LimitCount: limit,
	})
}

func (r *OutboxRepository) Complete(ctx context.Context, id pgtype.UUID, processedAt time.Time) (dbgen.CompleteOutboxEventRow, error) {
	if r == nil || r.queries == nil {
		return dbgen.CompleteOutboxEventRow{}, errors.New("outbox repository is not initialized")
	}
	processedAtTime, err := requiredDatabaseTime(processedAt, "processed time")
	if err != nil {
		return dbgen.CompleteOutboxEventRow{}, err
	}
	return r.queries.CompleteOutboxEvent(ctx, dbgen.CompleteOutboxEventParams{
		ID:          id,
		ProcessedAt: &processedAtTime,
	})
}

func (r *OutboxRepository) Fail(ctx context.Context, id pgtype.UUID, status string, nextAvailableAt time.Time, lastErrorCode string) (dbgen.FailOutboxEventRow, error) {
	if r == nil || r.queries == nil {
		return dbgen.FailOutboxEventRow{}, errors.New("outbox repository is not initialized")
	}
	nextAvail, err := requiredDatabaseTime(nextAvailableAt, "next available time")
	if err != nil {
		return dbgen.FailOutboxEventRow{}, err
	}
	return r.queries.FailOutboxEvent(ctx, dbgen.FailOutboxEventParams{
		ID:              id,
		Status:          status,
		NextAvailableAt: nextAvail,
		LastErrorCode:   strings.TrimSpace(lastErrorCode),
	})
}
