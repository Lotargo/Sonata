package provider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ModelProvider isolates cognitive modules from any upstream wire format.
type ModelProvider interface {
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
	Stream(context.Context, GenerateRequest) (<-chan StreamEvent, error)
	ListModels(context.Context) ([]ModelDescriptor, error)
}

type GenerateRequest struct {
	Model           string
	Messages        []Message
	Tools           []ToolDefinition
	ToolChoice      string
	Temperature     float64
	MaxOutputTokens int
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

type ToolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type GenerateResult struct {
	ID           string
	Model        string
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
	TotalTokens  int
}

type StreamEvent struct {
	Delta        string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        *Usage
	Done         bool
	Err          error
}

type ModelDescriptor struct {
	ID           string
	Object       string
	OwnedBy      string
	Protocol     string
	PrivacyClass string
	Enabled      bool
	ReservedFor  string
}

type ErrorCode string

const (
	CodeProviderUnavailable  ErrorCode = "PROVIDER_UNAVAILABLE"
	CodeProviderExhausted    ErrorCode = "PROVIDER_EXHAUSTED"
	CodeModelDisabled        ErrorCode = "MODEL_DISABLED"
	CodeModelDeprecated      ErrorCode = "MODEL_DEPRECATED"
	CodeModelUnavailable     ErrorCode = "MODEL_UNAVAILABLE"
	CodeModelProtocol        ErrorCode = "MODEL_PROTOCOL_ERROR"
	CodeModelTimeout         ErrorCode = "MODEL_TIMEOUT"
	CodeModelRateLimited     ErrorCode = "MODEL_RATE_LIMITED"
	CodeModelResponseInvalid ErrorCode = "MODEL_RESPONSE_INVALID"
)

// Error contains only normalized metadata. It deliberately never stores an
// upstream response body or credential.
type Error struct {
	Code       ErrorCode
	Model      string
	StatusCode int
	RetryAfter time.Duration
	cause      error
}

func (e *Error) Error() string {
	if e.Model == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: model %s", e.Code, e.Model)
}

func (e *Error) Unwrap() error { return e.cause }

func ErrorCodeOf(err error) (ErrorCode, bool) {
	var providerErr *Error
	if !errors.As(err, &providerErr) {
		return "", false
	}
	return providerErr.Code, true
}

func IsFallbackEligible(err error) bool {
	code, ok := ErrorCodeOf(err)
	if !ok {
		return false
	}
	switch code {
	case CodeModelUnavailable,
		CodeModelTimeout,
		CodeModelRateLimited,
		CodeModelResponseInvalid,
		CodeModelProtocol:
		return true
	default:
		return false
	}
}
