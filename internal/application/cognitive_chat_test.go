package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/database"
	"github.com/Lotargo/Sonata/internal/httpapi"
	"github.com/Lotargo/Sonata/internal/protected"
	"github.com/Lotargo/Sonata/internal/provider"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		t.Skip("DATABASE_URL is not set; skipping database integration test")
	}
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed to connect to integration DB: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type testToolExecutor struct{}

func (testToolExecutor) ExecuteTools(ctx context.Context, calls []cognition.ToolCall) ([]cognition.ToolResult, error) {
	results := make([]cognition.ToolResult, len(calls))
	for i, c := range calls {
		results[i] = cognition.ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Content:    `{"result":"test success"}`,
		}
	}
	return results, nil
}

func TestCognitiveChatServiceImplCompleteDirect(t *testing.T) {
	pool := openTestPool(t)

	// Clean up user on exit
	ownerID := "cc-owner-direct-" + time.Now().Format("20060102150405")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})

	runRepo, err := database.NewRunRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	manifestRepo, err := database.NewManifestRepository(pool, protected.DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	providerUsageRepo, err := database.NewProviderUsageRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := protected.Load(os.DirFS(filepath.Join("..", "..", "protected")), "registry.json")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := protected.NewManifestResolver(bundle, protected.DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Mock provider to return direct route first, then the final synthesis
	var generateMu sync.Mutex
	callCount := 0
	providerMock := &mockModelProvider{
		generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
			generateMu.Lock()
			defer generateMu.Unlock()
			callCount++
			if gr.Model == "nemotron-3-ultra-free" {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"route":"direct"}`,
					Usage: provider.Usage{
						InputTokens:  10,
						OutputTokens: 5,
					},
				}, nil
			}
			return provider.GenerateResult{
				Model:   gr.Model,
				Content: "Hello from direct route!",
				Usage: provider.Usage{
					InputTokens:  20,
					OutputTokens: 15,
				},
			}, nil
		},
	}

	adapter := setupTestRunnerAdapter(t, providerMock)
	adapter.usageRepo = providerUsageRepo

	service, err := NewCognitiveChatServiceImpl(adapter, runRepo, manifestRepo, bundle, resolver)
	if err != nil {
		t.Fatal(err)
	}

	req := CognitiveChatRequest{
		Identity: httpapi.RequestIdentity{
			UserID:    ownerID,
			ChatID:    "chat-direct",
			MessageID: "msg-direct",
		},
		Messages: []httpapi.ChatMessage{
			{Role: "user", Content: []byte(`"Help me directly"`)},
		},
		Emotion: cognition.EmotionReport{
			Text: "Emotion: Calm",
		},
	}

	var emitted []string
	emit := func(delta httpapi.ChatDelta) error {
		emitted = append(emitted, delta.Content)
		return nil
	}

	res, err := service.Complete(context.Background(), req, emit)
	if err != nil {
		t.Fatal(err)
	}

	if res.FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", res.FinishReason)
	}

	fullEmitted := strings.Join(emitted, "")
	if fullEmitted != "Hello from direct route!" {
		t.Errorf("emitted = %q, want Hello from direct route!", fullEmitted)
	}

	// Verify database entries
	var route string
	var status string
	err = pool.QueryRow(context.Background(), `
		SELECT route, status FROM sonata.cognitive_runs WHERE owner_id = $1 AND conversation_id = $2
	`, ownerID, "chat-direct").Scan(&route, &status)
	if err != nil {
		t.Fatal(err)
	}
	if route != "direct" || status != "OK" {
		t.Errorf("DB run: route=%q status=%q", route, status)
	}

	var providerUsageCount int
	err = pool.QueryRow(context.Background(), `
		SELECT count(*) FROM sonata.provider_usage WHERE owner_id = $1
	`, ownerID).Scan(&providerUsageCount)
	if err != nil {
		t.Fatal(err)
	}
	if providerUsageCount != 2 {
		t.Errorf("provider usage count = %d, want 2", providerUsageCount)
	}
}

func TestCognitiveChatServiceImplCompleteFull(t *testing.T) {
	pool := openTestPool(t)

	// Clean up user on exit
	ownerID := "cc-owner-full-" + time.Now().Format("20060102150405")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})

	runRepo, err := database.NewRunRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	manifestRepo, err := database.NewManifestRepository(pool, protected.DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	providerUsageRepo, err := database.NewProviderUsageRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := protected.Load(os.DirFS(filepath.Join("..", "..", "protected")), "registry.json")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := protected.NewManifestResolver(bundle, protected.DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Mock provider to return full route first, then raw/critical/summary reports, then tooling outputs
	var generateMu sync.Mutex
	providerMock := &mockModelProvider{
		generate: func(ctx context.Context, gr provider.GenerateRequest) (provider.GenerateResult, error) {
			generateMu.Lock()
			defer generateMu.Unlock()
			if gr.Model == "nemotron-3-ultra-free" && !strings.Contains(gr.Messages[0].Content, "summary") {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"route":"full"}`,
				}, nil
			}
			if len(gr.Tools) > 0 {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"preliminary_decision":"tooling","tool_calls":[{"id":"call-1","name":"web.search.langsearch","arguments":{"q":"test"}}]}`,
				}, nil
			}
			if strings.Contains(gr.Messages[0].Content, "raw") {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"content":"raw","confidence":0.9}`,
				}, nil
			}
			if strings.Contains(gr.Messages[0].Content, "critical") {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"content":"critical","confidence":0.8}`,
				}, nil
			}
			if strings.Contains(gr.Messages[0].Content, "summary") {
				return provider.GenerateResult{
					Model:   gr.Model,
					Content: `{"initial_position":"a","main_critique":"b","revised_position":"c","confidence":0.7}`,
				}, nil
			}
			return provider.GenerateResult{
				Model:   gr.Model,
				Content: "Hello from full route!",
			}, nil
		},
	}

	adapter := setupTestRunnerAdapter(t, providerMock)
	adapter.usageRepo = providerUsageRepo

	service, err := NewCognitiveChatServiceImpl(adapter, runRepo, manifestRepo, bundle, resolver)
	if err != nil {
		t.Fatal(err)
	}

	req := CognitiveChatRequest{
		Identity: httpapi.RequestIdentity{
			UserID:    ownerID,
			ChatID:    "chat-full",
			MessageID: "msg-full",
		},
		Messages: []httpapi.ChatMessage{
			{Role: "user", Content: []byte(`"Analyze fully"`)},
		},
		Emotion: cognition.EmotionReport{
			Text: "Emotion: Curious",
		},
	}

	var emitted []string
	emit := func(delta httpapi.ChatDelta) error {
		emitted = append(emitted, delta.Content)
		return nil
	}

	// We pass the tool executor in context
	ctx := context.WithValue(context.Background(), "tool_executor", testToolExecutor{})

	res, err := service.Complete(ctx, req, emit)
	if err != nil {
		t.Fatal(err)
	}

	if res.FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", res.FinishReason)
	}

	fullEmitted := strings.Join(emitted, "")
	if fullEmitted != "Hello from full route!" {
		t.Errorf("emitted = %q, want Hello from full route!", fullEmitted)
	}

	// Verify database entries
	var route string
	var status string
	err = pool.QueryRow(context.Background(), `
		SELECT route, status FROM sonata.cognitive_runs WHERE owner_id = $1 AND conversation_id = $2
	`, ownerID, "chat-full").Scan(&route, &status)
	if err != nil {
		t.Fatal(err)
	}
	if route != "full" || status != "OK" {
		t.Errorf("DB run: route=%q status=%q", route, status)
	}

	// Verify tool call was recorded
	var toolCount int
	err = pool.QueryRow(context.Background(), `
		SELECT count(*) FROM sonata.tool_calls WHERE owner_id = $1
	`, ownerID).Scan(&toolCount)
	if err != nil {
		t.Fatal(err)
	}
	if toolCount != 1 {
		t.Errorf("tool count = %d, want 1", toolCount)
	}
}
