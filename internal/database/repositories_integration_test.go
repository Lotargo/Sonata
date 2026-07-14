package database

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestInstructionRepositoryUpsertAndGet(t *testing.T) {
	pool := openIntegrationPool(t)
	repo, err := NewInstructionRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instructionID := fmt.Sprintf("test.instruction.%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.instruction_versions WHERE instruction_id = $1`, instructionID)
	})

	created, err := repo.Upsert(ctx, UpsertInstructionVersionInput{
		InstructionID: instructionID,
		Version:       1,
		ContentHash:   "hash1",
		Metadata:      json.RawMessage(`{"author":"test"}`),
		CreatedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert instruction version: %v", err)
	}
	if created.InstructionID != instructionID || created.Version != 1 || created.ContentHash != "hash1" {
		t.Fatalf("unexpected created instruction version: %#v", created)
	}

	fetched, err := repo.Get(ctx, instructionID, 1)
	if err != nil {
		t.Fatalf("get instruction version: %v", err)
	}
	if fetched.InstructionID != instructionID || fetched.Version != 1 || fetched.ContentHash != "hash1" {
		t.Fatalf("unexpected fetched instruction version: %#v", fetched)
	}

	// Test upsert updates
	updated, err := repo.Upsert(ctx, UpsertInstructionVersionInput{
		InstructionID: instructionID,
		Version:       1,
		ContentHash:   "hash2",
		Metadata:      json.RawMessage(`{"author":"test2"}`),
		CreatedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert instruction version update: %v", err)
	}
	if updated.ContentHash != "hash2" {
		t.Fatalf("unexpected updated hash: %s", updated.ContentHash)
	}
}

func TestOutboxRepositoryWorkflow(t *testing.T) {
	pool := openIntegrationPool(t)
	repo, err := NewOutboxRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := "outbox-owner-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, owner)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO sonata.users (id) VALUES ($1)`, owner); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	event, err := repo.Insert(ctx, InsertOutboxEventInput{
		OwnerID:       &owner,
		AggregateType: "User",
		AggregateID:   owner,
		EventType:     "UserCreated",
		Payload:       json.RawMessage(`{"name":"test"}`),
		Status:        "pending",
		Attempts:      0,
		AvailableAt:   now,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	var uuidVal pgtype.UUID
	_ = uuidVal.Scan(event.ID.String())

	// Test lock pending
	pending, err := repo.LockPending(ctx, now, now.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("lock pending: %v", err)
	}
	found := false
	for _, p := range pending {
		if p.ID == event.ID {
			found = true
			if p.Status != "processing" {
				t.Fatalf("locked event status = %s, want processing", p.Status)
			}
		}
	}
	if !found {
		t.Fatal("inserted outbox event not found in locked list")
	}

	// Test complete
	comp, err := repo.Complete(ctx, uuidVal, now)
	if err != nil {
		t.Fatalf("complete outbox event: %v", err)
	}
	if comp.Status != "completed" {
		t.Fatalf("completed status = %s", comp.Status)
	}

	// Lock again and fail
	event2, err := repo.Insert(ctx, InsertOutboxEventInput{
		OwnerID:       &owner,
		AggregateType: "User",
		AggregateID:   owner,
		EventType:     "UserUpdated",
		Payload:       json.RawMessage(`{"name":"test2"}`),
		Status:        "pending",
		Attempts:      0,
		AvailableAt:   now,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatal(err)
	}

	var uuidVal2 pgtype.UUID
	_ = uuidVal2.Scan(event2.ID.String())

	pending2, err := repo.LockPending(ctx, now, now.Add(time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, p := range pending2 {
		if p.ID == event2.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("second event not locked")
	}

	failed, err := repo.Fail(ctx, uuidVal2, "failed", now.Add(5*time.Minute), "ERR_TEST")
	if err != nil {
		t.Fatalf("fail outbox event: %v", err)
	}
	if failed.Status != "failed" {
		t.Fatalf("failed status = %s", failed.Status)
	}
}

func TestToolCallAndProviderUsageRepositories(t *testing.T) {
	pool := openIntegrationPool(t)
	runRepo, err := NewRunRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	toolRepo, err := NewToolCallRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	usageRepo, err := NewProviderUsageRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerID := "run-owner-" + suffix
	conversationID := "conv-" + suffix
	messageID := "msg-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})

	now := time.Now()
	begun, err := runRepo.BeginCognitiveRun(ctx, BeginCognitiveRunInput{
		OwnerID:           ownerID,
		ConversationID:    conversationID,
		ConversationTitle: "Integration title",
		MessageID:         messageID,
		MessageContent:    json.RawMessage(`"hello"`),
		Route:             cognition.RouteDirect,
		StartedAt:         now,
		Metadata:          json.RawMessage(`{}`),
		Roles: []RoleRunStart{
			{
				Role:        cognition.RoleSynthesisFinal,
				ModelID:     "primary-model",
				Instruction: cognition.ArtifactRef{ID: "synthesis.final", Version: 1, Hash: "hash-inst"},
				Manifest:    cognition.ManifestRef{ArtifactRef: cognition.ArtifactRef{ID: "manifest.final.default", Version: 1, Hash: "hash-man"}, Source: "system_default"},
			},
		},
	})
	if err != nil {
		t.Fatalf("begin cognitive run: %v", err)
	}

	var cogRunID pgtype.UUID
	_ = cogRunID.Scan(begun.Run.ID.String())
	var roleRunID pgtype.UUID
	_ = roleRunID.Scan(begun.Roles[0].ID.String())

	// Test ToolCall
	tc, err := toolRepo.Insert(ctx, InsertToolCallInput{
		OwnerID:         ownerID,
		CognitiveRunID:  cogRunID,
		RoleRunID:       roleRunID,
		ToolName:        "web.search",
		Status:          "PLANNED",
		RequestMetadata: json.RawMessage(`{"q":"test"}`),
		ResultMetadata:  json.RawMessage(`{}`),
		CreatedAt:       now,
	})
	if err != nil {
		t.Fatalf("insert tool call: %v", err)
	}
	if tc.ToolName != "web.search" || tc.Status != "PLANNED" {
		t.Fatalf("unexpected tool call insert: %#v", tc)
	}

	var tcID pgtype.UUID
	_ = tcID.Scan(tc.ID.String())

	comp, err := toolRepo.Complete(ctx, CompleteToolCallInput{
		OwnerID:        ownerID,
		CognitiveRunID: cogRunID,
		ToolCallID:     tcID,
		Status:         "OK",
		ResultMetadata: json.RawMessage(`{"res":"ok"}`),
		CompletedAt:    now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("complete tool call: %v", err)
	}
	if comp.Status != "OK" {
		t.Fatalf("unexpected completed tool call: %#v", comp)
	}

	list, err := toolRepo.List(ctx, ownerID, cogRunID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(list) != 1 || list[0].ID != tc.ID {
		t.Fatalf("list tool calls returned: %#v", list)
	}

	// Test ProviderUsage
	usage, err := usageRepo.Insert(ctx, InsertProviderUsageInput{
		OwnerID:        ownerID,
		CognitiveRunID: cogRunID,
		RoleRunID:      roleRunID,
		Provider:       "open_code_zen",
		ModelID:        "primary-model",
		InputTokens:    10,
		OutputTokens:   20,
		CachedTokens:   5,
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("insert provider usage: %v", err)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 20 || usage.CachedTokens != 5 {
		t.Fatalf("unexpected provider usage insert: %#v", usage)
	}
}
