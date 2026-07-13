package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testInternalCredential = "test-openwebui-credential"

type chatServiceFunc func(context.Context, ChatRequest, func(ChatDelta) error) (ChatResult, error)

func (f chatServiceFunc) Complete(ctx context.Context, request ChatRequest, emit func(ChatDelta) error) (ChatResult, error) {
	return f(ctx, request, emit)
}

func TestV1RequiresServiceCredentialBeforeForwardedIdentity(t *testing.T) {
	var calls atomic.Int32
	chat := chatServiceFunc(func(context.Context, ChatRequest, func(ChatDelta) error) (ChatResult, error) {
		calls.Add(1)
		return ChatResult{}, nil
	})
	handler := NewHandler(Options{InternalCredential: testInternalCredential, Chat: chat})

	spoofed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(true)))
	addForwardedIdentity(spoofed)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, spoofed)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed request status = %d, want 401", response.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("chat service was called before service authentication")
	}

	missingIdentity := authorizedRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(true)))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, missingIdentity)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing identity status = %d, want 400", response.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("chat service was called without forwarded identity")
	}
}

func TestChatCompletionsSSE(t *testing.T) {
	var received ChatRequest
	chat := chatServiceFunc(func(_ context.Context, request ChatRequest, emit func(ChatDelta) error) (ChatResult, error) {
		received = request
		for _, content := range []string{"Hello", " world"} {
			if err := emit(ChatDelta{Content: content}); err != nil {
				return ChatResult{}, err
			}
		}
		return ChatResult{FinishReason: "stop"}, nil
	})
	handler := NewHandler(Options{InternalCredential: testInternalCredential, Chat: chat})
	request := authorizedRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(true)))
	addForwardedIdentity(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("stream status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if received.Identity != (RequestIdentity{UserID: "user-1", ChatID: "chat-1", MessageID: "message-1"}) {
		t.Fatalf("forwarded identity = %#v", received.Identity)
	}
	body := response.Body.String()
	for _, fragment := range []string{
		`"object":"chat.completion.chunk"`,
		`"role":"assistant"`,
		`"content":"Hello"`,
		`"content":" world"`,
		`"finish_reason":"stop"`,
		"data: [DONE]\n\n",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("SSE body missing %q:\n%s", fragment, body)
		}
	}
}

func TestChatCompletionsNonStreaming(t *testing.T) {
	chat := chatServiceFunc(func(_ context.Context, _ ChatRequest, emit func(ChatDelta) error) (ChatResult, error) {
		if err := emit(ChatDelta{Content: "complete response"}); err != nil {
			return ChatResult{}, err
		}
		return ChatResult{}, nil
	})
	handler := NewHandler(Options{InternalCredential: testInternalCredential, Chat: chat})
	request := authorizedRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(false)))
	addForwardedIdentity(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("non-stream status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var payload chatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode completion: %v", err)
	}
	if len(payload.Choices) != 1 || payload.Choices[0].Message.Content != "complete response" {
		t.Fatalf("unexpected completion: %#v", payload)
	}
}

func TestRequestCancellationReachesChatService(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	chat := chatServiceFunc(func(ctx context.Context, _ ChatRequest, _ func(ChatDelta) error) (ChatResult, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return ChatResult{}, ctx.Err()
	})
	handler := NewHandler(Options{InternalCredential: testInternalCredential, Chat: chat})
	ctx, cancel := context.WithCancel(context.Background())
	request := authorizedRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(true))).WithContext(ctx)
	addForwardedIdentity(request)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("chat service did not start")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("chat service did not observe request cancellation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("HTTP handler did not return after cancellation")
	}
}

func authorizedRequest(method, path string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Authorization", "Bearer "+testInternalCredential)
	return request
}

func addForwardedIdentity(request *http.Request) {
	request.Header.Set(headerOpenWebUIUserID, "user-1")
	request.Header.Set(headerOpenWebUIChatID, "chat-1")
	request.Header.Set(headerOpenWebUIMessageID, "message-1")
}

func validChatBody(stream bool) string {
	return `{"model":"sonata","messages":[{"role":"user","content":"hello"}],"stream":` + map[bool]string{true: "true", false: "false"}[stream] + `}`
}
