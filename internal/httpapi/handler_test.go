package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestHealthAndModelsEndpoints(t *testing.T) {
	handler := NewHandler(Options{})

	for path := range map[string]struct{}{
		"/internal/health/live":  {},
		"/internal/health/ready": {},
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.Code)
		}
		if got := response.Header().Get("X-Request-ID"); got == "" {
			t.Fatalf("GET %s did not return X-Request-ID", path)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /v1/models status = %d, want 200", response.Code)
	}
	var payload modelList
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode model list: %v", err)
	}
	if payload.Object != "list" || len(payload.Data) != 1 || payload.Data[0].ID != modelID {
		t.Fatalf("unexpected model list: %#v", payload)
	}
}

func TestReadinessFailure(t *testing.T) {
	handler := NewHandler(Options{Ready: func(context.Context) error { return errors.New("database unavailable") }})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/internal/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want 503", response.Code)
	}
	if strings.Contains(response.Body.String(), "database unavailable") {
		t.Fatal("readiness response exposed internal dependency error")
	}
}

func TestChatCompletionsSkeleton(t *testing.T) {
	handler := NewHandler(Options{})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"sonata","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("chat skeleton status = %d, want 501", response.Code)
	}
	var payload apiErrorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode API error: %v", err)
	}
	if payload.Error.Type != "not_implemented" {
		t.Fatalf("error type = %q", payload.Error.Type)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON status = %d, want 400", response.Code)
	}
}

func TestStructuredAccessLog(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := NewHandler(Options{Logger: logger})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	logLine := output.String()
	for _, fragment := range []string{`"msg":"http request"`, `"method":"GET"`, `"path":"/v1/models"`, `"status":200`, `"request_id":"`} {
		if !strings.Contains(logLine, fragment) {
			t.Fatalf("structured access log missing %s: %s", fragment, logLine)
		}
	}
}

func TestRecoverPanics(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	panicHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	wrapped := chimiddleware.RequestID(requestIDHeader(accessLog(logger)(recoverPanics(logger)(panicHandler))))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", response.Code)
	}
	if !strings.Contains(output.String(), `"msg":"http handler panic"`) {
		t.Fatalf("panic was not logged: %s", output.String())
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(listener.Addr().String(), NewHandler(Options{}), time.Second, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	result := make(chan error, 1)
	go func() { result <- server.Serve(ctx, listener) }()

	client := &http.Client{Timeout: time.Second}
	url := "http://" + listener.Addr().String() + "/internal/health/live"
	deadline := time.Now().Add(2 * time.Second)
	for {
		response, requestErr := client.Get(url)
		if requestErr == nil {
			_ = response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready: %v", requestErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down gracefully")
	}
}
