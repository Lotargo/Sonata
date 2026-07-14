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
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	cognitiveRunStatusRunning = "RUNNING"
	roleRunStatusRunning      = "RUNNING"
)

type RunRepository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewRunRepository(pool *pgxpool.Pool) (*RunRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &RunRepository{pool: pool, queries: dbgen.New(pool)}, nil
}

type RoleRunStart struct {
	Role        cognition.RuntimeRole
	ModelID     string
	Instruction cognition.ArtifactRef
	Manifest    cognition.ManifestRef
}

type BeginCognitiveRunInput struct {
	OwnerID           string
	ConversationID    string
	ConversationTitle string
	MessageID         string
	MessageContent    json.RawMessage
	Route             cognition.Route
	StartedAt         time.Time
	Metadata          json.RawMessage
	Roles             []RoleRunStart
}

type BegunCognitiveRun struct {
	Run   dbgen.CognitiveRun
	Roles []dbgen.RoleRun
}

func (repository *RunRepository) BeginCognitiveRun(
	ctx context.Context,
	input BeginCognitiveRunInput,
) (BegunCognitiveRun, error) {
	if repository == nil || repository.pool == nil || repository.queries == nil {
		return BegunCognitiveRun{}, errors.New("run repository is not initialized")
	}
	if err := validateBeginCognitiveRunInput(input); err != nil {
		return BegunCognitiveRun{}, err
	}
	startedAt := input.StartedAt.UTC().Truncate(time.Microsecond)
	metadata := normalizedJSON(input.Metadata)

	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return BegunCognitiveRun{}, fmt.Errorf("begin cognitive run transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := repository.queries.WithTx(tx)

	if _, err := queries.EnsureUser(ctx, dbgen.EnsureUserParams{
		OwnerID:   strings.TrimSpace(input.OwnerID),
		UpdatedAt: startedAt,
	}); err != nil {
		return BegunCognitiveRun{}, fmt.Errorf("ensure cognitive run owner: %w", err)
	}
	if _, err := queries.UpsertConversation(ctx, dbgen.UpsertConversationParams{
		OwnerID:        strings.TrimSpace(input.OwnerID),
		ConversationID: strings.TrimSpace(input.ConversationID),
		Title:          strings.TrimSpace(input.ConversationTitle),
		CreatedAt:      startedAt,
		UpdatedAt:      startedAt,
	}); err != nil {
		return BegunCognitiveRun{}, fmt.Errorf("upsert cognitive conversation: %w", err)
	}
	if _, err := queries.InsertMessage(ctx, dbgen.InsertMessageParams{
		OwnerID:        strings.TrimSpace(input.OwnerID),
		ConversationID: strings.TrimSpace(input.ConversationID),
		MessageID:      strings.TrimSpace(input.MessageID),
		Role:           "user",
		Content:        append([]byte(nil), input.MessageContent...),
		CreatedAt:      startedAt,
	}); err != nil {
		return BegunCognitiveRun{}, fmt.Errorf("insert cognitive request message: %w", err)
	}

	run, err := queries.CreateCognitiveRun(ctx, dbgen.CreateCognitiveRunParams{
		OwnerID:          strings.TrimSpace(input.OwnerID),
		ConversationID:   strings.TrimSpace(input.ConversationID),
		RequestMessageID: strings.TrimSpace(input.MessageID),
		Route:            string(input.Route),
		Status:           cognitiveRunStatusRunning,
		StartedAt:        startedAt,
		Metadata:         metadata,
	})
	if err != nil {
		return BegunCognitiveRun{}, fmt.Errorf("create cognitive run: %w", err)
	}

	roleRuns := make([]dbgen.RoleRun, 0, len(input.Roles))
	for _, role := range input.Roles {
		spec, exists := runtimeRoleSpec(role.Role)
		if !exists {
			return BegunCognitiveRun{}, fmt.Errorf("unknown runtime role %q", role.Role)
		}
		created, err := queries.CreateRoleRun(ctx, dbgen.CreateRoleRunParams{
			OwnerID:            strings.TrimSpace(input.OwnerID),
			CognitiveRunID:     run.ID,
			Phase:              string(spec.Phase),
			Perspective:        spec.Perspective,
			Status:             roleRunStatusRunning,
			ModelID:            strings.TrimSpace(role.ModelID),
			InstructionID:      role.Instruction.ID,
			InstructionVersion: int32(role.Instruction.Version),
			InstructionHash:    role.Instruction.Hash,
			ManifestID:         role.Manifest.ID,
			ManifestVersion:    int32(role.Manifest.Version),
			ManifestHash:       role.Manifest.Hash,
			ManifestSource:     role.Manifest.Source,
			LatencyMs:          0,
			Usage:              []byte(`{}`),
			ErrorCode:          "",
			CreatedAt:          startedAt,
		})
		if err != nil {
			return BegunCognitiveRun{}, fmt.Errorf("create %s role run: %w", role.Role, err)
		}
		roleRuns = append(roleRuns, created)
	}

	if err := tx.Commit(ctx); err != nil {
		return BegunCognitiveRun{}, fmt.Errorf("commit cognitive run transaction: %w", err)
	}
	return BegunCognitiveRun{Run: run, Roles: roleRuns}, nil
}

func validateBeginCognitiveRunInput(input BeginCognitiveRunInput) error {
	if strings.TrimSpace(input.OwnerID) == "" {
		return errors.New("cognitive run owner ID is required")
	}
	if strings.TrimSpace(input.ConversationID) == "" {
		return errors.New("cognitive run conversation ID is required")
	}
	if strings.TrimSpace(input.MessageID) == "" {
		return errors.New("cognitive run message ID is required")
	}
	if !json.Valid(input.MessageContent) {
		return errors.New("cognitive run message content must be valid JSON")
	}
	if !input.Route.Valid() {
		return fmt.Errorf("invalid cognitive route %q", input.Route)
	}
	if input.StartedAt.IsZero() {
		return errors.New("cognitive run start time is required")
	}
	if len(input.Metadata) > 0 && !json.Valid(input.Metadata) {
		return errors.New("cognitive run metadata must be valid JSON")
	}
	if len(input.Roles) == 0 {
		return errors.New("cognitive run requires at least one role")
	}

	seenRoles := make(map[cognition.RuntimeRole]struct{}, len(input.Roles))
	for _, role := range input.Roles {
		if _, exists := seenRoles[role.Role]; exists {
			return fmt.Errorf("duplicate runtime role %q", role.Role)
		}
		seenRoles[role.Role] = struct{}{}
		spec, exists := runtimeRoleSpec(role.Role)
		if !exists {
			return fmt.Errorf("unknown runtime role %q", role.Role)
		}
		if strings.TrimSpace(role.ModelID) == "" {
			return fmt.Errorf("role %s model ID is required", role.Role)
		}
		if role.Instruction.Version < 1 || role.Instruction.Version > math.MaxInt32 {
			return fmt.Errorf("role %s instruction version is invalid", role.Role)
		}
		if strings.TrimSpace(role.Instruction.ID) == "" || strings.TrimSpace(role.Instruction.Hash) == "" {
			return fmt.Errorf("role %s instruction metadata is incomplete", role.Role)
		}
		if role.Instruction.ID != spec.InstructionID {
			return fmt.Errorf(
				"role %s instruction ID is %q, want %q",
				role.Role,
				role.Instruction.ID,
				spec.InstructionID,
			)
		}
		if role.Manifest.Version < 1 || role.Manifest.Version > math.MaxInt32 {
			return fmt.Errorf("role %s manifest version is invalid", role.Role)
		}
		if strings.TrimSpace(role.Manifest.ID) == "" ||
			strings.TrimSpace(role.Manifest.Hash) == "" ||
			strings.TrimSpace(role.Manifest.Source) == "" {
			return fmt.Errorf("role %s manifest metadata is incomplete", role.Role)
		}
		switch role.Manifest.Source {
		case "system_default", "user_global", "user_chat":
		default:
			return fmt.Errorf("role %s manifest source %q is invalid", role.Role, role.Manifest.Source)
		}
	}
	return nil
}

func runtimeRoleSpec(role cognition.RuntimeRole) (cognition.RoleSpec, bool) {
	for _, spec := range cognition.RuntimeRoleSpecs() {
		if spec.Role == role {
			return spec, true
		}
	}
	return cognition.RoleSpec{}, false
}

func normalizedJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return append([]byte(nil), value...)
}
