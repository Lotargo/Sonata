package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/emotion"
	"github.com/Lotargo/Sonata/internal/httpapi"
)

// AffectiveRuntime is the application-facing subset of the deterministic
// affective engine.
type AffectiveRuntime interface {
	ProcessUserMessage(context.Context, string, string) (emotion.AffectiveReport, error)
	DegradedReport(string) (emotion.AffectiveReport, error)
}

// CognitiveChatRequest is the explicit boundary between HTTP transport and the
// cognitive chat implementation. One canonical report is attached before any
// cognitive role executes.
type CognitiveChatRequest struct {
	Identity httpapi.RequestIdentity
	Model    string
	Messages []httpapi.ChatMessage
	Emotion  cognition.EmotionReport
}

type CognitiveChatService interface {
	Complete(context.Context, CognitiveChatRequest, func(httpapi.ChatDelta) error) (httpapi.ChatResult, error)
}

// AffectiveChatService implements httpapi.ChatService and enriches each trusted
// HTTP request before handing it to the cognitive application service.
type AffectiveChatService struct {
	runtime AffectiveRuntime
	next    CognitiveChatService
}

func NewAffectiveChatService(runtime AffectiveRuntime, next CognitiveChatService) (*AffectiveChatService, error) {
	if runtime == nil {
		return nil, errors.New("affective runtime is required")
	}
	if next == nil {
		return nil, errors.New("cognitive chat service is required")
	}
	return &AffectiveChatService{runtime: runtime, next: next}, nil
}

func (service *AffectiveChatService) Complete(
	ctx context.Context,
	request httpapi.ChatRequest,
	emit func(httpapi.ChatDelta) error,
) (httpapi.ChatResult, error) {
	if service == nil || service.runtime == nil || service.next == nil {
		return httpapi.ChatResult{}, errors.New("affective chat service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return httpapi.ChatResult{}, err
	}
	userID := strings.TrimSpace(request.Identity.UserID)
	if userID == "" {
		return httpapi.ChatResult{}, errors.New("trusted user ID is required")
	}

	report, err := service.runtime.ProcessUserMessage(ctx, userID, latestUserText(request.Messages))
	if err != nil {
		if ctx.Err() != nil {
			return httpapi.ChatResult{}, ctx.Err()
		}
		report, err = service.runtime.DegradedReport(userID)
		if err != nil {
			return httpapi.ChatResult{}, fmt.Errorf("build degraded affective report: %w", err)
		}
	}
	if err := report.Validate(); err != nil {
		return httpapi.ChatResult{}, fmt.Errorf("validate affective report: %w", err)
	}
	canonical := cognition.EmotionReport{
		Text:         report.Text,
		StateVersion: report.StateVersion,
	}
	if err := canonical.Validate(); err != nil {
		return httpapi.ChatResult{}, fmt.Errorf("validate canonical emotion report: %w", err)
	}

	return service.next.Complete(ctx, CognitiveChatRequest{
		Identity: request.Identity,
		Model:    request.Model,
		Messages: cloneChatMessages(request.Messages),
		Emotion:  canonical,
	}, emit)
}

func latestUserText(messages []httpapi.ChatMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if !strings.EqualFold(strings.TrimSpace(messages[index].Role), "user") {
			continue
		}
		var text string
		if err := json.Unmarshal(messages[index].Content, &text); err == nil {
			return text
		}
		// Structured multimodal content remains available to the cognitive
		// service, but is not interpreted as an affective lexical signal.
		return ""
	}
	return ""
}

func cloneChatMessages(messages []httpapi.ChatMessage) []httpapi.ChatMessage {
	cloned := make([]httpapi.ChatMessage, len(messages))
	for index, message := range messages {
		cloned[index] = message
		cloned[index].Content = append(json.RawMessage(nil), message.Content...)
	}
	return cloned
}
