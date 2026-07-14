package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
)

func TestRunRepositoryCompletesRolesBeforeCognitiveRun(t *testing.T) {
	pool := openIntegrationPool(t)
	repository, err := NewRunRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerID := "completion-owner-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})
	startedAt := time.Date(2026, time.July, 14, 16, 0, 0, 0, time.UTC)
	roleStarts := []RoleRunStart{
		canonicalRoleStart(cognition.RoleRouter),
		canonicalRoleStart(cognition.RoleSynthesisFinal),
	}
	begun, err := repository.BeginCognitiveRun(context.Background(), BeginCognitiveRunInput{
		OwnerID:        ownerID,
		ConversationID: "completion-chat-" + suffix,
		MessageID:      "completion-message-" + suffix,
		MessageContent: json.RawMessage(`"hello"`),
		Route:          cognition.RouteDirect,
		StartedAt:      startedAt,
		Roles:          roleStarts,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = repository.CompleteCognitiveRun(context.Background(), CompleteCognitiveRunInput{
		OwnerID:        ownerID,
		CognitiveRunID: begun.Run.ID,
		Status:         CognitiveRunStatusOK,
		CompletedAt:    startedAt.Add(time.Second),
	})
	if !errors.Is(err, ErrCognitiveRunNotCompletable) {
		t.Fatalf("early cognitive completion error = %v, want not completable", err)
	}

	wrongMetadata := completedRoleMetadata(roleStarts[0], cognition.RoleStatusSucceeded)
	wrongMetadata.Manifest.Hash = "wrong-manifest-hash"
	_, err = repository.CompleteRoleRun(context.Background(), CompleteRoleRunInput{
		OwnerID:        ownerID,
		CognitiveRunID: begun.Run.ID,
		RoleRunID:      begun.Roles[0].ID,
		Metadata:       wrongMetadata,
		Usage:          json.RawMessage(`{"input_tokens":1}`),
	})
	if !errors.Is(err, ErrRoleRunNotCompletable) {
		t.Fatalf("mismatched role completion error = %v, want not completable", err)
	}

	for index, start := range roleStarts {
		completed, err := repository.CompleteRoleRun(context.Background(), CompleteRoleRunInput{
			OwnerID:        ownerID,
			CognitiveRunID: begun.Run.ID,
			RoleRunID:      begun.Roles[index].ID,
			Metadata:       completedRoleMetadata(start, cognition.RoleStatusSucceeded),
			Usage:          json.RawMessage(`{"input_tokens":1,"output_tokens":1}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if completed.Status != "OK" || completed.LatencyMs != 25 || completed.ModelID != "completed-model" {
			t.Fatalf("completed role = %#v", completed)
		}
	}

	_, err = repository.CompleteRoleRun(context.Background(), CompleteRoleRunInput{
		OwnerID:        ownerID,
		CognitiveRunID: begun.Run.ID,
		RoleRunID:      begun.Roles[0].ID,
		Metadata:       completedRoleMetadata(roleStarts[0], cognition.RoleStatusSucceeded),
	})
	if !errors.Is(err, ErrRoleRunNotCompletable) {
		t.Fatalf("duplicate role completion error = %v, want not completable", err)
	}

	completedAt := startedAt.Add(2 * time.Second)
	completedRun, err := repository.CompleteCognitiveRun(context.Background(), CompleteCognitiveRunInput{
		OwnerID:        ownerID,
		CognitiveRunID: begun.Run.ID,
		Status:         CognitiveRunStatusOK,
		CompletedAt:    completedAt,
		Metadata:       json.RawMessage(`{"final":"ok"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if completedRun.Status != string(CognitiveRunStatusOK) || completedRun.CompletedAt == nil || !completedRun.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed cognitive run = %#v", completedRun)
	}

	_, err = repository.CompleteCognitiveRun(context.Background(), CompleteCognitiveRunInput{
		OwnerID:        ownerID,
		CognitiveRunID: begun.Run.ID,
		Status:         CognitiveRunStatusOK,
		CompletedAt:    completedAt.Add(time.Second),
	})
	if !errors.Is(err, ErrCognitiveRunNotCompletable) {
		t.Fatalf("duplicate cognitive completion error = %v, want not completable", err)
	}
}

func completedRoleMetadata(start RoleRunStart, status cognition.RoleStatus) cognition.RoleMetadata {
	return cognition.RoleMetadata{
		Role:        start.Role,
		Status:      status,
		Latency:     25 * time.Millisecond,
		ModelID:     "completed-model",
		Instruction: start.Instruction,
		Manifest:    start.Manifest,
	}
}
