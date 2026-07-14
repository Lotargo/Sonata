package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/database/dbgen"
	protectedcore "github.com/Lotargo/Sonata/internal/protected"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserManifestNotFound = errors.New("user manifest not found")

type ManifestRepository struct {
	pool             *pgxpool.Pool
	queries          *dbgen.Queries
	maxManifestBytes int
}

func NewManifestRepository(pool *pgxpool.Pool, maxManifestBytes int) (*ManifestRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	if maxManifestBytes <= 0 {
		return nil, errors.New("manifest byte limit must be positive")
	}
	return &ManifestRepository{
		pool:             pool,
		queries:          dbgen.New(pool),
		maxManifestBytes: maxManifestBytes,
	}, nil
}

type PutUserManifestInput struct {
	OwnerID string
	Scope   protectedcore.ManifestScope
	ScopeID string
	Content string
	At      time.Time
}

func (repository *ManifestRepository) Put(
	ctx context.Context,
	input PutUserManifestInput,
) (protectedcore.UserManifest, error) {
	if repository == nil || repository.pool == nil || repository.queries == nil {
		return protectedcore.UserManifest{}, errors.New("manifest repository is not initialized")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return protectedcore.UserManifest{}, errors.New("manifest owner ID is required")
	}
	scope, scopeID, source, err := normalizeManifestScope(input.Scope, input.ScopeID)
	if err != nil {
		return protectedcore.UserManifest{}, err
	}
	content, err := protectedcore.NormalizeUserManifest(input.Content, repository.maxManifestBytes)
	if err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("normalize user manifest: %w", err)
	}
	at, err := requiredDatabaseTime(input.At, "manifest update time")
	if err != nil {
		return protectedcore.UserManifest{}, err
	}
	return repository.writeVersion(ctx, manifestVersionWrite{
		OwnerID: ownerID,
		Scope:   scope,
		ScopeID: scopeID,
		Source:  source,
		Status:  protectedcore.ManifestStatusActive,
		Content: content,
		At:      at,
	})
}

func (repository *ManifestRepository) Delete(
	ctx context.Context,
	ownerID string,
	scope protectedcore.ManifestScope,
	scopeID string,
	at time.Time,
) (protectedcore.UserManifest, error) {
	if repository == nil || repository.pool == nil || repository.queries == nil {
		return protectedcore.UserManifest{}, errors.New("manifest repository is not initialized")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return protectedcore.UserManifest{}, errors.New("manifest owner ID is required")
	}
	normalizedScope, normalizedScopeID, source, err := normalizeManifestScope(scope, scopeID)
	if err != nil {
		return protectedcore.UserManifest{}, err
	}
	at, err = requiredDatabaseTime(at, "manifest delete time")
	if err != nil {
		return protectedcore.UserManifest{}, err
	}
	return repository.writeVersion(ctx, manifestVersionWrite{
		OwnerID: ownerID,
		Scope:   normalizedScope,
		ScopeID: normalizedScopeID,
		Source:  source,
		Status:  protectedcore.ManifestStatusDeleted,
		Content: "",
		At:      at,
	})
}

func (repository *ManifestRepository) Get(
	ctx context.Context,
	ownerID string,
	scope protectedcore.ManifestScope,
	scopeID string,
) (protectedcore.UserManifest, error) {
	if repository == nil || repository.queries == nil {
		return protectedcore.UserManifest{}, errors.New("manifest repository is not initialized")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return protectedcore.UserManifest{}, errors.New("manifest owner ID is required")
	}
	normalizedScope, normalizedScopeID, _, err := normalizeManifestScope(scope, scopeID)
	if err != nil {
		return protectedcore.UserManifest{}, err
	}
	stored, err := repository.queries.GetUserManifest(ctx, dbgen.GetUserManifestParams{
		OwnerID: ownerID,
		Scope:   string(normalizedScope),
		ScopeID: normalizedScopeID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return protectedcore.UserManifest{}, ErrUserManifestNotFound
	}
	if err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("get user manifest: %w", err)
	}
	return userManifestFromRow(stored)
}

type manifestVersionWrite struct {
	OwnerID string
	Scope   protectedcore.ManifestScope
	ScopeID string
	Source  protectedcore.ManifestSource
	Status  protectedcore.ManifestStatus
	Content string
	At      time.Time
}

func (repository *ManifestRepository) writeVersion(
	ctx context.Context,
	write manifestVersionWrite,
) (protectedcore.UserManifest, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("begin manifest transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := repository.queries.WithTx(tx)

	if _, err := queries.EnsureUser(ctx, dbgen.EnsureUserParams{
		OwnerID:   write.OwnerID,
		UpdatedAt: write.At,
	}); err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("ensure manifest owner: %w", err)
	}
	lockParams := dbgen.LockUserManifestScopeParams{
		OwnerID: write.OwnerID,
		Scope:   string(write.Scope),
		ScopeID: write.ScopeID,
	}
	if err := queries.LockUserManifestScope(ctx, lockParams); err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("lock manifest scope: %w", err)
	}

	current, err := queries.GetUserManifestForUpdate(ctx, dbgen.GetUserManifestForUpdateParams(lockParams))
	manifestID := ""
	version := int32(1)
	createdAt := write.At
	switch {
	case err == nil:
		if current.Version == math.MaxInt32 {
			return protectedcore.UserManifest{}, errors.New("user manifest version limit reached")
		}
		manifestID = current.ManifestID
		version = current.Version + 1
		createdAt = current.CreatedAt
	case errors.Is(err, pgx.ErrNoRows):
		if write.Status == protectedcore.ManifestStatusDeleted {
			return protectedcore.UserManifest{}, ErrUserManifestNotFound
		}
		manifestID, err = queries.NewManifestID(ctx)
		if err != nil {
			return protectedcore.UserManifest{}, fmt.Errorf("allocate user manifest ID: %w", err)
		}
	default:
		return protectedcore.UserManifest{}, fmt.Errorf("load current user manifest: %w", err)
	}

	contentHash := hashManifestContent(write.Content)
	metadata, err := json.Marshal(struct {
		Status protectedcore.ManifestStatus `json:"status"`
	}{Status: write.Status})
	if err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("encode manifest version metadata: %w", err)
	}
	ownerID := write.OwnerID
	if _, err := queries.InsertManifestVersion(ctx, dbgen.InsertManifestVersionParams{
		ManifestID:  manifestID,
		Version:     version,
		OwnerID:     &ownerID,
		Source:      string(write.Source),
		ContentHash: contentHash,
		Metadata:    metadata,
		CreatedAt:   write.At,
	}); err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("insert manifest version: %w", err)
	}
	stored, err := queries.UpsertUserManifest(ctx, dbgen.UpsertUserManifestParams{
		OwnerID:     write.OwnerID,
		Scope:       string(write.Scope),
		ScopeID:     write.ScopeID,
		ManifestID:  manifestID,
		Version:     version,
		Status:      string(write.Status),
		Content:     write.Content,
		ContentHash: contentHash,
		CreatedAt:   createdAt,
		UpdatedAt:   write.At,
	})
	if err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("upsert current user manifest: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return protectedcore.UserManifest{}, fmt.Errorf("commit manifest transaction: %w", err)
	}
	return userManifestFromRow(stored)
}

func normalizeManifestScope(
	scope protectedcore.ManifestScope,
	scopeID string,
) (protectedcore.ManifestScope, string, protectedcore.ManifestSource, error) {
	scopeID = strings.TrimSpace(scopeID)
	switch scope {
	case protectedcore.ManifestScopeGlobal:
		if scopeID != "" {
			return "", "", "", errors.New("global manifest must not have a scope ID")
		}
		return scope, "", protectedcore.ManifestSourceUserGlobal, nil
	case protectedcore.ManifestScopeChat:
		if scopeID == "" {
			return "", "", "", errors.New("chat manifest scope ID is required")
		}
		return scope, scopeID, protectedcore.ManifestSourceUserChat, nil
	default:
		return "", "", "", fmt.Errorf("unsupported manifest scope %q", scope)
	}
}

func userManifestFromRow(row dbgen.UserManifest) (protectedcore.UserManifest, error) {
	status := protectedcore.ManifestStatus(row.Status)
	switch status {
	case protectedcore.ManifestStatusActive,
		protectedcore.ManifestStatusDisabled,
		protectedcore.ManifestStatusDeleted,
		protectedcore.ManifestStatusRejected:
	default:
		return protectedcore.UserManifest{}, fmt.Errorf("stored manifest status %q is invalid", row.Status)
	}
	if row.Version < 1 {
		return protectedcore.UserManifest{}, errors.New("stored manifest version is invalid")
	}
	return protectedcore.UserManifest{
		Metadata: protectedcore.Metadata{
			ID:      row.ManifestID,
			Version: int(row.Version),
			Hash:    row.ContentHash,
		},
		OwnerID: row.OwnerID,
		Scope:   protectedcore.ManifestScope(row.Scope),
		ScopeID: row.ScopeID,
		Status:  status,
		Content: row.Content,
	}, nil
}

func hashManifestContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func requiredDatabaseTime(value time.Time, label string) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, fmt.Errorf("%s is required", label)
	}
	return value.UTC().Truncate(time.Microsecond), nil
}
