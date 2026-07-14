package database

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
)

func TestRunRepositoryBeginsCanonicalRunAtomically(t *testing.T) {
	pool := openIntegrationPool(t)
	repository, err := NewRunRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerID := "run-owner-" + suffix
	conversationID := "run-conversation-" + suffix
	messageID := "run-message-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})
	startedAt := time.Date(2026, time.July, 14, 13, 0, 0, 123456000, time.UTC)

	begun, err := repository.BeginCognitiveRun(context.Background(), BeginCognitiveRunInput{
		OwnerID:          ownerID,
		ConversationID:   conversationID,
		ConversationTitle: "Canonical transaction",
		MessageID:        messageID,
		MessageContent:   json.RawMessage(`"hello"`),
		Route:            cognition.RouteDirect,
		StartedAt:        startedAt,
		Metadata:         json.RawMessage(`{"request_id":"request-1"}`),
		Roles: []RoleRunStart{
			canonicalRoleStart(cognition.RoleRouter),
			canonicalRoleStart(cognition.RoleSynthesisFinal),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if begun.Run.OwnerID != ownerID || begun.Run.ConversationID != conversationID || begun.Run.RequestMessageID != messageID {
		t.Fatalf("canonical run = %#v", begun.Run)
	}
	if begun.Run.Status != cognitiveRunStatusRunning || begun.Run.Route != string(cognition.RouteDirect) {
		t.Fatalf("canonical run status/route = %#v", begun.Run)
	}
	if len(begun.Roles) != 2 {
		t.Fatalf("role runs = %d, want 2", len(begun.Roles))
	}
	if begun.Roles[0].Phase != string(cognition.PhaseRouter) || begun.Roles[1].Phase != string(cognition.PhaseSynthesisFinal) {
		t.Fatalf("role phases = %#v", begun.Roles)
	}

	var counts struct {
		users        int
		conversations int
		messages     int
		runs         int
		roles        int
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM sonata.users WHERE id = $1),
			(SELECT count(*) FROM sonata.conversations WHERE owner_id = $1 AND id = $2),
			(SELECT count(*) FROM sonata.messages WHERE owner_id = $1 AND id = $3),
			(SELECT count(*) FROM sonata.cognitive_runs WHERE owner_id = $1 AND conversation_id = $2),
			(SELECT count(*) FROM sonata.role_runs WHERE owner_id = $1 AND cognitive_run_id = $4)
	`, ownerID, conversationID, messageID, begun.Run.ID).Scan(
		&counts.users,
		&counts.conversations,
		&counts.messages,
		&counts.runs,
		&counts.roles,
	); err != nil {
		t.Fatal(err)
	}
	if counts.users != 1 || counts.conversations != 1 || counts.messages != 1 || counts.runs != 1 || counts.roles != 2 {
		t.Fatalf("canonical counts = %#v", counts)
	}
}

func TestRunRepositoryRollsBackPartialRequestOnMessageConflict(t *testing.T) {
	pool := openIntegrationPool(t)
	repository, err := NewRunRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerID := "rollback-owner-" + suffix
	existingConversationID := "existing-conversation-" + suffix
	newConversationID := "rolled-back-conversation-" + suffix
	messageID := "duplicate-message-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(context.Background(), `INSERT INTO sonata.users (id) VALUES ($1)`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO sonata.conversations (owner_id, id)
		VALUES ($1, $2)
	`, ownerID, existingConversationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO sonata.messages (owner_id, conversation_id, id, role, content)
		VALUES ($1, $2, $3, 'user', '"existing"'::jsonb)
	`, ownerID, existingConversationID, messageID); err != nil {
		t.Fatal(err)
	}

	_, err = repository.BeginCognitiveRun(context.Background(), BeginCognitiveRunInput{
		OwnerID:          ownerID,
		ConversationID:   newConversationID,
		ConversationTitle: "Must roll back",
		MessageID:        messageID,
		MessageContent:   json.RawMessage(`"duplicate"`),
		Route:            cognition.RouteDirect,
		StartedAt:        time.Now().UTC(),
		Roles:            []RoleRunStart{canonicalRoleStart(cognition.RoleRouter)},
	})
	if err == nil {
		t.Fatal("duplicate message unexpectedly committed")
	}

	var conversationCount, runCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM sonata.conversations WHERE owner_id = $1 AND id = $2),
			(SELECT count(*) FROM sonata.cognitive_runs WHERE owner_id = $1 AND conversation_id = $2)
	`, ownerID, newConversationID).Scan(&conversationCount, &runCount); err != nil {
		t.Fatal(err)
	}
	if conversationCount != 0 || runCount != 0 {
		t.Fatalf("partial transaction persisted: conversations=%d runs=%d", conversationCount, runCount)
	}
}

func canonicalRoleStart(role cognition.RuntimeRole) RoleRunStart {
	spec, _ := runtimeRoleSpec(role)
	return RoleRunStart{
		Role:    role,
		ModelID: "test-model",
		Instruction: cognition.ArtifactRef{
			ID:      spec.InstructionID,
			Version: 1,
			Hash:    "instruction-hash",
		},
		Manifest: cognition.ManifestRef{
			ArtifactRef: cognition.ArtifactRef{
				ID:      "manifest.default",
				Version: 1,
				Hash:    "manifest-hash",
			},
			Source: "system_default",
		},
	}
}
