package application

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
	"github.com/Lotargo/Sonata/internal/emotion"
	"github.com/Lotargo/Sonata/internal/httpapi"
)

const applicationTestCredential = "application-test-credential"

type cognitiveChatServiceFunc func(context.Context, CognitiveChatRequest, func(httpapi.ChatDelta) error) (httpapi.ChatResult, error)

func (function cognitiveChatServiceFunc) Complete(
	ctx context.Context,
	request CognitiveChatRequest,
	emit func(httpapi.ChatDelta) error,
) (httpapi.ChatResult, error) {
	return function(ctx, request, emit)
}

func TestHTTPChatBuildsOneVersionedAffectiveReportBeforeCognition(t *testing.T) {
	profile := loadApplicationAffectiveProfile(t)
	now := time.Date(2026, time.July, 14, 9, 0, 0, 0, time.UTC)
	store := emotion.NewMemoryAffectiveStateStore()
	runtime, err := emotion.NewAffectiveRuntime("sonata", profile, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	var received []CognitiveChatRequest
	next := cognitiveChatServiceFunc(func(_ context.Context, request CognitiveChatRequest, emit func(httpapi.ChatDelta) error) (httpapi.ChatResult, error) {
		received = append(received, request)
		if err := emit(httpapi.ChatDelta{Content: "ok"}); err != nil {
			return httpapi.ChatResult{}, err
		}
		return httpapi.ChatResult{FinishReason: "stop"}, nil
	})
	chat, err := NewAffectiveChatService(runtime, next)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(httpapi.Options{InternalCredential: applicationTestCredential, Chat: chat})

	response := performChatRequest(handler, "user-1", "message-1", "СПАСИБО!!!")
	if response.Code != http.StatusOK {
		t.Fatalf("first response status = %d: %s", response.Code, response.Body.String())
	}
	if len(received) != 1 {
		t.Fatalf("cognitive calls = %d, want 1", len(received))
	}
	first := received[0]
	if first.Identity.UserID != "user-1" || first.Emotion.StateVersion != 1 {
		t.Fatalf("first cognitive request = %#v", first)
	}
	if !strings.Contains(first.Emotion.Text, "status=HEALTHY") || strings.Contains(strings.ToLower(first.Emotion.Text), "спасибо") {
		t.Fatalf("unsafe or invalid emotion report: %q", first.Emotion.Text)
	}

	now = now.Add(time.Hour)
	response = performChatRequest(handler, "user-1", "message-2", "ненавижу")
	if response.Code != http.StatusOK {
		t.Fatalf("second response status = %d: %s", response.Code, response.Body.String())
	}
	if len(received) != 2 || received[1].Emotion.StateVersion != 2 {
		t.Fatalf("second cognitive request = %#v", received)
	}

	response = performChatRequest(handler, "user-2", "message-3", "спасибо")
	if response.Code != http.StatusOK {
		t.Fatalf("other-user response status = %d: %s", response.Code, response.Body.String())
	}
	if len(received) != 3 || received[2].Emotion.StateVersion != 1 {
		t.Fatalf("owner isolation failed: %#v", received)
	}
	if received[0].Messages[0].Content[0] != '"' {
		t.Fatal("cognitive message content was unexpectedly mutated")
	}
}

func TestHTTPChatContinuesWithDegradedReportOnAffectiveStoreFailure(t *testing.T) {
	profile := loadApplicationAffectiveProfile(t)
	now := time.Date(2026, time.July, 14, 9, 0, 0, 0, time.UTC)
	runtime, err := emotion.NewAffectiveRuntime("sonata", profile, unavailableAffectiveStore{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	var received CognitiveChatRequest
	next := cognitiveChatServiceFunc(func(_ context.Context, request CognitiveChatRequest, emit func(httpapi.ChatDelta) error) (httpapi.ChatResult, error) {
		received = request
		if err := emit(httpapi.ChatDelta{Content: "degraded but available"}); err != nil {
			return httpapi.ChatResult{}, err
		}
		return httpapi.ChatResult{}, nil
	})
	chat, err := NewAffectiveChatService(runtime, next)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(httpapi.Options{InternalCredential: applicationTestCredential, Chat: chat})
	response := performChatRequest(handler, "user-1", "message-1", "спасибо")
	if response.Code != http.StatusOK {
		t.Fatalf("response status = %d: %s", response.Code, response.Body.String())
	}
	if received.Emotion.StateVersion != 0 || !strings.Contains(received.Emotion.Text, "status=DEGRADED") {
		t.Fatalf("degraded cognitive request = %#v", received)
	}
}

func TestAffectiveChatServicePropagatesCancellationWithoutCallingCognition(t *testing.T) {
	profile := loadApplicationAffectiveProfile(t)
	runtime, err := emotion.NewAffectiveRuntime("sonata", profile, emotion.NewMemoryAffectiveStateStore(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	chat, err := NewAffectiveChatService(runtime, cognitiveChatServiceFunc(func(context.Context, CognitiveChatRequest, func(httpapi.ChatDelta) error) (httpapi.ChatResult, error) {
		called = true
		return httpapi.ChatResult{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = chat.Complete(ctx, httpapi.ChatRequest{Identity: httpapi.RequestIdentity{UserID: "user-1"}}, func(httpapi.ChatDelta) error { return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("error=%v called=%t", err, called)
	}
}

func performChatRequest(handler http.Handler, userID, messageID, text string) *httptest.ResponseRecorder {
	body := `{"model":"sonata","messages":[{"role":"user","content":` + quoteJSON(text) + `}],"stream":false}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+applicationTestCredential)
	request.Header.Set("X-OpenWebUI-User-Id", userID)
	request.Header.Set("X-OpenWebUI-Chat-Id", "chat-1")
	request.Header.Set("X-OpenWebUI-Message-Id", messageID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func quoteJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + replacer.Replace(value) + `"`
}

func loadApplicationAffectiveProfile(t *testing.T) emotion.AffectiveRuntimeProfile {
	t.Helper()
	for key, value := range map[string]string{
		"OPENCODE_ZEN_API_KEY":      "zen-secret",
		"DATABASE_URL":              "postgres://pool",
		"DATABASE_DIRECT_URL":       "postgres://direct",
		"LANGSEARCH_API_KEY":        "lang-secret",
		"QDRANT_URL":                "https://qdrant.example.test",
		"QDRANT_API_KEY":            "qdrant-secret",
		"OPENWEBUI_INTERNAL_SECRET": "internal-secret",
	} {
		t.Setenv(key, value)
	}
	runtimeConfig, err := config.NewLoader(nil).Load(context.Background(), filepath.Join("..", "..", "config"), "local")
	if err != nil {
		t.Fatalf("load application config: %v", err)
	}
	profile, err := emotion.NewAffectiveRuntimeProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatalf("build affective runtime profile: %v", err)
	}
	return profile
}

type unavailableAffectiveStore struct{}

func (unavailableAffectiveStore) Load(context.Context, emotion.StateKey) (emotion.AffectiveState, bool, error) {
	return emotion.AffectiveState{}, false, errors.New("store unavailable")
}

func (unavailableAffectiveStore) CompareAndSwap(context.Context, emotion.StateKey, int64, emotion.AffectiveState) error {
	return errors.New("store unavailable")
}
