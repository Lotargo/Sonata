package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/database/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrRoleRunNotCompletable      = errors.New("role run is not completable")
	ErrCognitiveRunNotCompletable = errors.New("cognitive run is not completable")
)

type CognitiveRunFinalStatus string

const (
	CognitiveRunStatusOK                CognitiveRunFinalStatus = "OK"
	CognitiveRunStatusDegraded          CognitiveRunFinalStatus = "DEGRADED"
	CognitiveRunStatusProviderExhausted CognitiveRunFinalStatus = "PROVIDER_EXHAUSTED"
	CognitiveRunStatusFailedRouting     CognitiveRunFinalStatus = "FAILED_ROUTING"
	CognitiveRunStatusFailedContext     CognitiveRunFinalStatus = "FAILED_CONTEXT"
	CognitiveRunStatusFailedTooling     CognitiveRunFinalStatus = "FAILED_TOOLING"
	CognitiveRunStatusFailedSynthesis   CognitiveRunFinalStatus = "FAILED_SYNTHESIS"
)

func (status CognitiveRunFinalStatus) Valid() bool {
	switch status {
	case CognitiveRunStatusOK,
		CognitiveRunStatusDegraded,
		CognitiveRunStatusProviderExhausted,
		CognitiveRunStatusFailedRouting,
		CognitiveRunStatusFailedContext,
		CognitiveRunStatusFailedTooling,
		CognitiveRunStatusFailedSynthesis:
		return true
	default:
		return false
	}
}

type CompleteRoleRunInput struct {
	OwnerID        string
	CognitiveRunID pgtype.UUID
	RoleRunID      pgtype.UUID
	Metadata       cognition.RoleMetadata
	Usage          json.RawMessage
	ErrorCode      string
}

func (repository *RunRepository) CompleteRoleRun(
	ctx context.Context,
	input CompleteRoleRunInput,
) (dbgen.RoleRun, error) {
	if repository == nil || repository.queries == nil {
		return dbgen.RoleRun{}, errors.New("run repository is not initialized")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return dbgen.RoleRun{}, errors.New("role run owner ID is required")
	}
	if !input.CognitiveRunID.Valid || !input.RoleRunID.Valid {
		return dbgen.RoleRun{}, errors.New("cognitive run ID and role run ID are required")
	}
	spec, status, err := validateRoleCompletionMetadata(input.Metadata)
	if err != nil {
		return dbgen.RoleRun{}, err
	}
	usage := normalizedJSON(input.Usage)
	if !json.Valid(usage) {
		return dbgen.RoleRun{}, errors.New("role usage must be valid JSON")
	}

	completed, err := repository.queries.CompleteRoleRun(ctx, dbgen.CompleteRoleRunParams{
		Status:                     status,
		ModelID:                    strings.TrimSpace(input.Metadata.ModelID),
		LatencyMs:                  input.Metadata.Latency.Milliseconds(),
		Usage:                      usage,
		ErrorCode:                  strings.TrimSpace(input.ErrorCode),
		OwnerID:                    ownerID,
		CognitiveRunID:             input.CognitiveRunID,
		RoleRunID:                  input.RoleRunID,
		ExpectedPhase:              string(spec.Phase),
		ExpectedPerspective:        spec.Perspective,
		ExpectedInstructionID:      input.Metadata.Instruction.ID,
		ExpectedInstructionVersion: int32(input.Metadata.Instruction.Version),
		ExpectedInstructionHash:    input.Metadata.Instruction.Hash,
		ExpectedManifestID:         input.Metadata.Manifest.ID,
		ExpectedManifestVersion:    int32(input.Metadata.Manifest.Version),
		ExpectedManifestHash:       input.Metadata.Manifest.Hash,
		ExpectedManifestSource:     input.Metadata.Manifest.Source,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.RoleRun{}, ErrRoleRunNotCompletable
	}
	if err != nil {
		return dbgen.RoleRun{}, fmt.Errorf("complete role run: %w", err)
	}
	return completed, nil
}

type CompleteCognitiveRunInput struct {
	OwnerID        string
	CognitiveRunID pgtype.UUID
	Status         CognitiveRunFinalStatus
	CompletedAt    time.Time
	Metadata       json.RawMessage
}

func (repository *RunRepository) CompleteCognitiveRun(
	ctx context.Context,
	input CompleteCognitiveRunInput,
) (dbgen.CognitiveRun, error) {
	if repository == nil || repository.queries == nil {
		return dbgen.CognitiveRun{}, errors.New("run repository is not initialized")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return dbgen.CognitiveRun{}, errors.New("cognitive run owner ID is required")
	}
	if !input.CognitiveRunID.Valid {
		return dbgen.CognitiveRun{}, errors.New("cognitive run ID is required")
	}
	if !input.Status.Valid() {
		return dbgen.CognitiveRun{}, fmt.Errorf("invalid cognitive run final status %q", input.Status)
	}
	completedAt, err := requiredDatabaseTime(input.CompletedAt, "cognitive run completion time")
	if err != nil {
		return dbgen.CognitiveRun{}, err
	}
	metadata := normalizedJSON(input.Metadata)
	if !json.Valid(metadata) {
		return dbgen.CognitiveRun{}, errors.New("cognitive run metadata must be valid JSON")
	}

	completed, err := repository.queries.CompleteCognitiveRun(ctx, dbgen.CompleteCognitiveRunParams{
		Status:         string(input.Status),
		CompletedAt:    completedAt,
		Metadata:       metadata,
		OwnerID:        ownerID,
		CognitiveRunID: input.CognitiveRunID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.CognitiveRun{}, ErrCognitiveRunNotCompletable
	}
	if err != nil {
		return dbgen.CognitiveRun{}, fmt.Errorf("complete cognitive run: %w", err)
	}
	return completed, nil
}

func validateRoleCompletionMetadata(metadata cognition.RoleMetadata) (cognition.RoleSpec, string, error) {
	spec, exists := runtimeRoleSpec(metadata.Role)
	if !exists {
		return cognition.RoleSpec{}, "", fmt.Errorf("unknown runtime role %q", metadata.Role)
	}
	status := ""
	switch metadata.Status {
	case cognition.RoleSucceeded:
		status = "OK"
	case cognition.RoleDegraded:
		status = "DEGRADED"
	case cognition.RoleFailed:
		status = "FAILED"
	default:
		return cognition.RoleSpec{}, "", fmt.Errorf("role %s status %q is invalid", metadata.Role, metadata.Status)
	}
	if metadata.Latency < 0 {
		return cognition.RoleSpec{}, "", fmt.Errorf("role %s latency cannot be negative", metadata.Role)
	}
	if strings.TrimSpace(metadata.ModelID) == "" {
		return cognition.RoleSpec{}, "", fmt.Errorf("role %s model ID is required", metadata.Role)
	}
	if metadata.Instruction.ID != spec.InstructionID ||
		metadata.Instruction.Version < 1 ||
		metadata.Instruction.Version > math.MaxInt32 ||
		strings.TrimSpace(metadata.Instruction.Hash) == "" {
		return cognition.RoleSpec{}, "", fmt.Errorf("role %s instruction metadata is invalid", metadata.Role)
	}
	if metadata.Manifest.Version < 1 ||
		metadata.Manifest.Version > math.MaxInt32 ||
		strings.TrimSpace(metadata.Manifest.ID) == "" ||
		strings.TrimSpace(metadata.Manifest.Hash) == "" {
		return cognition.RoleSpec{}, "", fmt.Errorf("role %s manifest metadata is invalid", metadata.Role)
	}
	switch metadata.Manifest.Source {
	case "system_default", "user_global", "user_chat":
	default:
		return cognition.RoleSpec{}, "", fmt.Errorf("role %s manifest source %q is invalid", metadata.Role, metadata.Manifest.Source)
	}
	return spec, status, nil
}
