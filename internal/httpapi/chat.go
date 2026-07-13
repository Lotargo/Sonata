package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var ErrChatUnavailable = errors.New("chat service unavailable")

type ChatService interface {
	Complete(context.Context, ChatRequest, func(ChatDelta) error) (ChatResult, error)
}

type ChatRequest struct {
	Identity RequestIdentity
	Model    string
	Messages []ChatMessage
}

type ChatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Name    string          `json:"name,omitempty"`
}

type ChatDelta struct {
	Content string
}

type ChatResult struct {
	FinishReason string
}

func chatCompletionsHandler(maxRequestBytes int64, chat ChatService, outputFilter OutputFilterFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, ok := decodeChatCompletionRequest(w, r, maxRequestBytes)
		if !ok {
			return
		}
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "trusted request identity is unavailable")
			return
		}
		if chat == nil {
			writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "chat completion pipeline is not connected yet")
			return
		}

		input := ChatRequest{Identity: identity, Model: request.Model, Messages: request.Messages}
		if request.Stream {
			streamChatCompletion(w, r, chat, input, outputFilter)
			return
		}
		writeChatCompletion(w, r, chat, input, outputFilter)
	}
}

func decodeChatCompletionRequest(w http.ResponseWriter, r *http.Request, maxRequestBytes int64) (chatCompletionRequest, bool) {
	reader := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer reader.Close()

	var request chatCompletionRequest
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "request body must be valid JSON")
		return chatCompletionRequest{}, false
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "request body must contain one JSON object")
		return chatCompletionRequest{}, false
	}
	if request.Model == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return chatCompletionRequest{}, false
	}
	if request.Model != modelID {
		writeAPIError(w, http.StatusNotFound, "model_not_found", "requested model is not available")
		return chatCompletionRequest{}, false
	}
	if len(request.Messages) == 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "messages are required")
		return chatCompletionRequest{}, false
	}
	for _, message := range request.Messages {
		if strings.TrimSpace(message.Role) == "" || len(message.Content) == 0 || string(message.Content) == "null" {
			writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "each message requires role and content")
			return chatCompletionRequest{}, false
		}
	}
	return request, true
}

func streamChatCompletion(w http.ResponseWriter, r *http.Request, chat ChatService, input ChatRequest, outputFilter OutputFilterFactory) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming_unsupported", "response streaming is unavailable")
		return
	}

	completionID := newCompletionID()
	created := time.Now().Unix()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if err := writeSSEJSON(w, flusher, chatCompletionChunk{
		ID: completionID, Object: "chat.completion.chunk", Created: created, Model: modelID,
		Choices: []chatCompletionChunkChoice{{Index: 0, Delta: chatCompletionDelta{Role: "assistant"}}},
	}); err != nil {
		return
	}

	filter := newOutputFilter(outputFilter)
	writeContent := func(content string) error {
		if content == "" {
			return nil
		}
		return writeSSEJSON(w, flusher, chatCompletionChunk{
			ID: completionID, Object: "chat.completion.chunk", Created: created, Model: modelID,
			Choices: []chatCompletionChunkChoice{{Index: 0, Delta: chatCompletionDelta{Content: content}}},
		})
	}
	result, err := chat.Complete(r.Context(), input, func(delta ChatDelta) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		safe, err := filter.Push(delta.Content)
		if err != nil {
			return err
		}
		return writeContent(safe)
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		_ = writeSSEJSON(w, flusher, apiErrorEnvelope{Error: apiError{Message: publicChatError(err), Type: "chat_completion_failed"}})
		_ = writeSSEDone(w, flusher)
		return
	}
	tail, err := filter.Close()
	if err != nil {
		_ = writeSSEJSON(w, flusher, apiErrorEnvelope{Error: apiError{Message: publicChatError(err), Type: "chat_completion_failed"}})
		_ = writeSSEDone(w, flusher)
		return
	}
	if err := writeContent(tail); err != nil {
		return
	}

	finishReason := result.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	if err := writeSSEJSON(w, flusher, chatCompletionChunk{
		ID: completionID, Object: "chat.completion.chunk", Created: created, Model: modelID,
		Choices: []chatCompletionChunkChoice{{Index: 0, Delta: chatCompletionDelta{}, FinishReason: &finishReason}},
	}); err != nil {
		return
	}
	_ = writeSSEDone(w, flusher)
}

func writeChatCompletion(w http.ResponseWriter, r *http.Request, chat ChatService, input ChatRequest, outputFilter OutputFilterFactory) {
	filter := newOutputFilter(outputFilter)
	var content strings.Builder
	result, err := chat.Complete(r.Context(), input, func(delta ChatDelta) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		safe, err := filter.Push(delta.Content)
		if err != nil {
			return err
		}
		_, writeErr := content.WriteString(safe)
		return writeErr
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		status := http.StatusBadGateway
		errorType := "chat_completion_failed"
		if errors.Is(err, ErrChatUnavailable) {
			status = http.StatusServiceUnavailable
			errorType = "service_unavailable"
		}
		writeAPIError(w, status, errorType, publicChatError(err))
		return
	}
	tail, err := filter.Close()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "chat_completion_failed", publicChatError(err))
		return
	}
	_, _ = content.WriteString(tail)

	finishReason := result.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	writeJSON(w, http.StatusOK, chatCompletionResponse{
		ID: newCompletionID(), Object: "chat.completion", Created: time.Now().Unix(), Model: modelID,
		Choices: []chatCompletionChoice{{
			Index:        0,
			Message:      chatCompletionMessage{Role: "assistant", Content: content.String()},
			FinishReason: finishReason,
		}},
	})
}

func publicChatError(err error) string {
	if errors.Is(err, ErrChatUnavailable) {
		return "chat completion service is unavailable"
	}
	return "chat completion failed"
}

func writeSSEJSON(w http.ResponseWriter, flusher http.Flusher, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEDone(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func newCompletionID() string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err == nil {
		return "chatcmpl-" + hex.EncodeToString(data[:])
	}
	return fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

type chatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []chatCompletionChunkChoice `json:"choices"`
}

type chatCompletionChunkChoice struct {
	Index        int                 `json:"index"`
	Delta        chatCompletionDelta `json:"delta"`
	FinishReason *string             `json:"finish_reason"`
}

type chatCompletionDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      chatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
