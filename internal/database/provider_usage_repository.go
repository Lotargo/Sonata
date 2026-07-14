package database

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/database/dbgen"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProviderUsageRepository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewProviderUsageRepository(pool *pgxpool.Pool) (*ProviderUsageRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &ProviderUsageRepository{pool: pool, queries: dbgen.New(pool)}, nil
}

type InsertProviderUsageInput struct {
	OwnerID        string
	CognitiveRunID pgtype.UUID
	RoleRunID      pgtype.UUID
	Provider       string
	ModelID        string
	InputTokens    int64
	OutputTokens   int64
	CachedTokens   int64
	CreatedAt      time.Time
}

func (r *ProviderUsageRepository) Insert(ctx context.Context, input InsertProviderUsageInput) (dbgen.ProviderUsage, error) {
	if r == nil || r.queries == nil {
		return dbgen.ProviderUsage{}, errors.New("provider usage repository is not initialized")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return dbgen.ProviderUsage{}, errors.New("owner ID is required")
	}
	if !input.CognitiveRunID.Valid || !input.RoleRunID.Valid {
		return dbgen.ProviderUsage{}, errors.New("cognitive run ID and role run ID are required")
	}
	if strings.TrimSpace(input.Provider) == "" || strings.TrimSpace(input.ModelID) == "" {
		return dbgen.ProviderUsage{}, errors.New("provider and model ID are required")
	}
	createdAt, err := requiredDatabaseTime(input.CreatedAt, "provider usage creation time")
	if err != nil {
		return dbgen.ProviderUsage{}, err
	}

	return r.queries.InsertProviderUsage(ctx, dbgen.InsertProviderUsageParams{
		OwnerID:        ownerID,
		CognitiveRunID: input.CognitiveRunID,
		RoleRunID:      input.RoleRunID,
		Provider:       input.Provider,
		ModelID:        input.ModelID,
		InputTokens:    input.InputTokens,
		OutputTokens:   input.OutputTokens,
		CachedTokens:   input.CachedTokens,
		CreatedAt:      createdAt,
	})
}
