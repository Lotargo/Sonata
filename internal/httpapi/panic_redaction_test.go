package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRecoverPanicsDoesNotLogPayload(t *testing.T) {
	const sensitive = "private-compiled-prompt-content"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	panicHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(sensitive) })
	wrapped := chimiddleware.RequestID(requestIDHeader(accessLog(logger)(recoverPanics(logger)(panicHandler))))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", response.Code)
	}
	logLine := output.String()
	if strings.Contains(logLine, sensitive) {
		t.Fatalf("panic payload leaked into logs: %s", logLine)
	}
	if !strings.Contains(logLine, `"panic_type":"string"`) {
		t.Fatalf("panic type missing from logs: %s", logLine)
	}
}
