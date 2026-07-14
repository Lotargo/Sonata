package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
	"github.com/Lotargo/Sonata/internal/emotion"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCanonicalSchemaRejectsCrossOwnerMessage(t *testing.T) {
	pool := openIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerOne := "owner-one-" + suffix
	ownerTwo := "owner-two-" + suffix
	conversationID := "conversation-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = ANY($1)`, []string{ownerOne, ownerTwo})
	})

	if _, err := pool.Exec(ctx, `INSERT INTO sonata.users (id) VALUES ($1), ($2)`, ownerOne, ownerTwo); err != nil {
		t.Fatalf("insert owners: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sonata.conversations (owner_id, id)
		VALUES ($1, $2)
	`, ownerOne, conversationID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO sonata.messages (owner_id, conversation_id, id, role, content)
		VALUES ($1, $2, $3, 'user', '{"text":"forbidden"}'::jsonb)
	`, ownerTwo, conversationID, "message-"+suffix)
	if err == nil {
		t.Fatal("cross-owner message insert unexpectedly succeeded")
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23503" {
		t.Fatalf("cross-owner insert error = %v, want foreign-key violation", err)
	}
}

func TestPostgresAffectiveStoreCompareAndSwap(t *testing.T) {
	pool := openIntegrationPool(t)
	store, err := NewPostgresAffectiveStateStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	profile := loadIntegrationAffectiveProfile(t)
	start := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	key := emotion.StateKey{
		IdentityID: "sonata",
		UserID:     fmt.Sprintf("affective-owner-%d", time.Now().UnixNano()),
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, key.UserID)
	})

	baseline, err := emotion.NewBaselineAffectiveStateFromProfiles(
		key,
		profile.Dynamics,
		profile.Initial,
		emotion.BaselineRelationshipState(),
		start,
	)
	if err != nil {
		t.Fatal(err)
	}
	first := baseline.Clone()
	first.Version = 1
	first.LastUpdatedAt = start.Add(time.Minute)
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSwap(context.Background(), key, 0, first); err != nil {
		t.Fatalf("insert first state: %v", err)
	}

	loaded, exists, err := store.Load(context.Background(), key)
	if err != nil || !exists {
		t.Fatalf("load first state: exists=%t err=%v", exists, err)
	}
	if loaded.Version != 1 || loaded.Key != key || !loaded.LastUpdatedAt.Equal(first.LastUpdatedAt) {
		t.Fatalf("loaded first state = %#v", loaded)
	}
	if err := store.CompareAndSwap(context.Background(), key, 0, first); !errors.Is(err, emotion.ErrVersionConflict) {
		t.Fatalf("stale insert error = %v, want version conflict", err)
	}

	second := first.Clone()
	second.Version = 2
	second.LastUpdatedAt = start.Add(2 * time.Minute)
	second.Relationship.Tension = 0.25
	if err := second.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSwap(context.Background(), key, 1, second); err != nil {
		t.Fatalf("update second state: %v", err)
	}
	loaded, exists, err = store.Load(context.Background(), key)
	if err != nil || !exists || loaded.Version != 2 || loaded.Relationship.Tension != 0.25 {
		t.Fatalf("load second state: state=%#v exists=%t err=%v", loaded, exists, err)
	}

	var eventCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM sonata.affective_events
		WHERE identity_id = $1 AND owner_id = $2
	`, key.IdentityID, key.UserID).Scan(&eventCount); err != nil {
		t.Fatalf("count affective events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("affective event count = %d, want 2", eventCount)
	}
}

func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("SONATA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SONATA_TEST_DATABASE_URL is not configured")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping integration database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func loadIntegrationAffectiveProfile(t *testing.T) emotion.AffectiveRuntimeProfile {
	t.Helper()
	for key, value := range map[string]string{
		"OPENCODE_ZEN_API_KEY":      "integration-zen",
		"DATABASE_URL":              "postgres://integration-runtime",
		"DATABASE_DIRECT_URL":       "postgres://integration-direct",
		"LANGSEARCH_API_KEY":        "integration-langsearch",
		"QDRANT_URL":                "https://qdrant.example.test",
		"QDRANT_API_KEY":            "integration-qdrant",
		"OPENWEBUI_INTERNAL_SECRET": "integration-openwebui",
	} {
		t.Setenv(key, value)
	}
	runtimeConfig, err := config.NewLoader(nil).Load(
		context.Background(),
		filepath.Join("..", "..", "config"),
		"local",
	)
	if err != nil {
		t.Fatalf("load integration config: %v", err)
	}
	profile, err := emotion.NewAffectiveRuntimeProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatalf("build integration affective profile: %v", err)
	}
	return profile
}
