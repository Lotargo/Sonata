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

type ToolCallRepository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewToolCallRepository(pool *pgxpool.Pool) (*ToolCallRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &ToolCallRepository{pool: pool, queries: dbgen.New(pool)}, nil
}

type InsertToolCallInput struct {
	OwnerID         string
	CognitiveRunID  pgtype.UUID
	RoleRunID       pgtype.UUID
	ToolName        string
	Status          string // 'PLANNED', 'RUNNING', 'OK', 'FAILED', 'REJECTED'
	RequestMetadata json.RawMessage
	ResultMetadata  json.RawMessage
	CreatedAt       time.Time
}

func (r *ToolCallRepository) Insert(ctx context.Context, input InsertToolCallInput) (dbgen.ToolCall, error) {
	if r == nil || r.queries == nil {
		return dbgen.ToolCall{}, errors.New("tool call repository is not initialized")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return dbgen.ToolCall{}, errors.New("owner ID is required")
	}
	if !input.CognitiveRunID.Valid || !input.RoleRunID.Valid {
		return dbgen.ToolCall{}, errors.New("cognitive run ID and role run ID are required")
	}
	if strings.TrimSpace(input.ToolName) == "" {
		return dbgen.ToolCall{}, errors.New("tool name is required")
	}
	createdAt, err := requiredDatabaseTime(input.CreatedAt, "tool call creation time")
	if err != nil {
		return dbgen.ToolCall{}, err
	}
	reqMeta := normalizedJSON(input.RequestMetadata)
	resMeta := normalizedJSON(input.ResultMetadata)

	return r.queries.InsertToolCall(ctx, dbgen.InsertToolCallParams{
		OwnerID:         ownerID,
		CognitiveRunID:  input.CognitiveRunID,
		RoleRunID:       input.RoleRunID,
		ToolName:        input.ToolName,
		Status:          input.Status,
		RequestMetadata: reqMeta,
		ResultMetadata:  resMeta,
		CreatedAt:       createdAt,
	})
}

type CompleteToolCallInput struct {
	OwnerID        string
	CognitiveRunID pgtype.UUID
	ToolCallID     pgtype.UUID
	Status         string
	ResultMetadata json.RawMessage
	CompletedAt    time.Time
}

func (r *ToolCallRepository) Complete(ctx context.Context, input CompleteToolCallInput) (dbgen.ToolCall, error) {
	if r == nil || r.queries == nil {
		return dbgen.ToolCall{}, errors.New("tool call repository is not initialized")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return dbgen.ToolCall{}, errors.New("owner ID is required")
	}
	if !input.CognitiveRunID.Valid || !input.ToolCallID.Valid {
		return dbgen.ToolCall{}, errors.New("cognitive run ID and tool call ID are required")
	}
	completedAt, err := requiredDatabaseTime(input.CompletedAt, "tool call completion time")
	if err != nil {
		return dbgen.ToolCall{}, err
	}
	resMeta := normalizedJSON(input.ResultMetadata)

	return r.queries.CompleteToolCall(ctx, dbgen.CompleteToolCallParams{
		OwnerID:        ownerID,
		CognitiveRunID: input.CognitiveRunID,
		ToolCallID:     input.ToolCallID,
		Status:         input.Status,
		ResultMetadata: resMeta,
		CompletedAt:    &completedAt,
	})
}

func (r *ToolCallRepository) List(ctx context.Context, ownerID string, cognitiveRunID pgtype.UUID) ([]dbgen.ToolCall, error) {
	if r == nil || r.queries == nil {
		return nil, errors.New("tool call repository is not initialized")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}
	if !cognitiveRunID.Valid {
		return nil, errors.New("cognitive run ID is required")
	}
	return r.queries.ListToolCalls(ctx, dbgen.ListToolCallsParams{
		OwnerID:        ownerID,
		CognitiveRunID: cognitiveRunID,
	})
}
