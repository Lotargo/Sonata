package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/database/dbgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InstructionRepository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewInstructionRepository(pool *pgxpool.Pool) (*InstructionRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &InstructionRepository{pool: pool, queries: dbgen.New(pool)}, nil
}

type UpsertInstructionVersionInput struct {
	InstructionID string
	Version       int32
	ContentHash   string
	Metadata      json.RawMessage
	CreatedAt     time.Time
}

func (r *InstructionRepository) Upsert(ctx context.Context, input UpsertInstructionVersionInput) (dbgen.InstructionVersion, error) {
	if r == nil || r.queries == nil {
		return dbgen.InstructionVersion{}, errors.New("instruction repository is not initialized")
	}
	instructionID := strings.TrimSpace(input.InstructionID)
	if instructionID == "" {
		return dbgen.InstructionVersion{}, errors.New("instruction ID is required")
	}
	if input.Version <= 0 {
		return dbgen.InstructionVersion{}, errors.New("instruction version must be positive")
	}
	if strings.TrimSpace(input.ContentHash) == "" {
		return dbgen.InstructionVersion{}, errors.New("content hash is required")
	}
	createdAt, err := requiredDatabaseTime(input.CreatedAt, "instruction creation time")
	if err != nil {
		return dbgen.InstructionVersion{}, err
	}
	metadata := normalizedJSON(input.Metadata)

	return r.queries.UpsertInstructionVersion(ctx, dbgen.UpsertInstructionVersionParams{
		InstructionID: instructionID,
		Version:       input.Version,
		ContentHash:   input.ContentHash,
		Metadata:      metadata,
		CreatedAt:     createdAt,
	})
}

func (r *InstructionRepository) Get(ctx context.Context, instructionID string, version int32) (dbgen.InstructionVersion, error) {
	if r == nil || r.queries == nil {
		return dbgen.InstructionVersion{}, errors.New("instruction repository is not initialized")
	}
	instructionID = strings.TrimSpace(instructionID)
	if instructionID == "" {
		return dbgen.InstructionVersion{}, errors.New("instruction ID is required")
	}
	if version <= 0 {
		return dbgen.InstructionVersion{}, errors.New("instruction version must be positive")
	}
	return r.queries.GetInstructionVersion(ctx, dbgen.GetInstructionVersionParams{
		InstructionID: instructionID,
		Version:       version,
	})
}
