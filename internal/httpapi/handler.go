package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const (
	modelID               = "sonata"
	defaultRequestTimeout = 2 * time.Minute
	defaultMaxRequestSize = 2 << 20
)

type ReadinessCheck func(context.Context) error

type Options struct {
	Logger             *slog.Logger
	RequestTimeout     time.Duration
	MaxRequestBytes    int64
	Ready              ReadinessCheck
	InternalCredential string
	Chat               ChatService
}

func NewHandler(options Options) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	requestTimeout := options.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	maxRequestBytes := options.MaxRequestBytes
	if maxRequestBytes <= 0 {
		maxRequestBytes = defaultMaxRequestSize
	}
	ready := options.Ready
	if ready == nil {
		ready = func(context.Context) error { return nil }
	}

	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(requestIDHeader)
	router.Use(accessLog(logger))
	router.Use(recoverPanics(logger))
	router.Use(chimiddleware.Timeout(requestTimeout))

	router.Get("/internal/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/internal/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := ready(r.Context()); err != nil {
			logger.WarnContext(r.Context(), "readiness check failed", "request_id", chimiddleware.GetReqID(r.Context()))
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Route("/v1", func(api chi.Router) {
		api.Use(requireInternalCredential(options.InternalCredential))
		api.Get("/models", modelsHandler)
		api.With(requireForwardedIdentity).Post("/chat/completions", chatCompletionsHandler(maxRequestBytes, options.Chat))
	})

	return router
}

func modelsHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, modelList{
		Object: "list",
		Data: []model{{
			ID:      modelID,
			Object:  "model",
			Created: 0,
			OwnedBy: "sonata",
		}},
	})
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestID := chimiddleware.GetReqID(r.Context()); requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}
		next.ServeHTTP(w, r)
	})
}

func recoverPanics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "http handler panic",
						"request_id", chimiddleware.GetReqID(r.Context()),
						"panic_type", fmt.Sprintf("%T", recovered),
						"stack", string(debug.Stack()),
					)
					if recorder, ok := w.(*responseRecorder); !ok || !recorder.wroteHeader {
						writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.InfoContext(r.Context(), "http request",
				"request_id", chimiddleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"bytes", recorder.bytes,
				"duration_ms", time.Since(started).Milliseconds(),
			)
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(data)
	w.bytes += written
	return written, err
}

func (w *responseRecorder) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, errorType, message string) {
	writeJSON(w, status, apiErrorEnvelope{Error: apiError{Message: message, Type: errorType}})
}

type modelList struct {
	Object string  `json:"object"`
	Data   []model `json:"data"`
}

type model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type apiErrorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}
