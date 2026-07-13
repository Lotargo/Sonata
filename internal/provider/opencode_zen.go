package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

const (
	openAIChatCompletionsSuffix = "/chat/completions"
	maxProviderResponseBytes    = 8 << 20
	maxProviderErrorBytes       = 64 << 10
	maxSSEEventBytes            = 1 << 20
)

type secretCredential interface {
	Reveal() string
	Empty() bool
}

type OpenCodeZenProvider struct {
	chatEndpoint   string
	modelsEndpoint string
	credential     secretCredential
	client         *http.Client
	allowlist      map[string]ModelDescriptor
}

// NewOpenCodeZenProvider is the production constructor. The resolved key enters
// the provider boundary as config.SecretValue and is never included in errors.
func NewOpenCodeZenProvider(
	endpoint string,
	credential config.SecretValue,
	client *http.Client,
	allowlist map[string]ModelDescriptor,
) (*OpenCodeZenProvider, error) {
	return newOpenCodeZenProvider(endpoint, credential, client, allowlist)
}

// NewOpenCodeZenProviderFromConfig accepts only the typed provider fragment,
// resolved credential and model definitions required by this adapter.
func NewOpenCodeZenProviderFromConfig(
	endpoint config.EndpointConfig,
	credential config.SecretValue,
	client *http.Client,
	models map[string]config.ModelDefinition,
) (*OpenCodeZenProvider, error) {
	if endpoint.Protocol != "openai_chat_completions" {
		return nil, errors.New("OpenCode Zen requires the openai_chat_completions protocol")
	}
	allowlist := make(map[string]ModelDescriptor)
	for id, definition := range models {
		if definition.ProviderRef != "open_code_zen" {
			continue
		}
		allowlist[id] = ModelDescriptor{
			ID:           id,
			Protocol:     definition.Protocol,
			PrivacyClass: definition.PrivacyClass,
			Enabled:      definition.Enabled,
			ReservedFor:  definition.ReservedFor,
		}
	}
	return NewOpenCodeZenProvider(endpoint.Endpoint, credential, client, allowlist)
}

func newOpenCodeZenProvider(
	endpoint string,
	credential secretCredential,
	client *http.Client,
	allowlist map[string]ModelDescriptor,
) (*OpenCodeZenProvider, error) {
	chatEndpoint, modelsEndpoint, err := normalizeZenEndpoints(endpoint)
	if err != nil {
		return nil, err
	}
	if credential == nil || credential.Empty() {
		return nil, errors.New("OpenCode Zen credential is required")
	}
	if client == nil {
		client = NewSharedHTTPClient()
	}
	if len(allowlist) == 0 {
		return nil, errors.New("OpenCode Zen model allowlist is required")
	}
	approved := make(map[string]ModelDescriptor, len(allowlist))
	for id, descriptor := range allowlist {
		if strings.TrimSpace(id) == "" || descriptor.ID != id {
			return nil, errors.New("OpenCode Zen allowlist contains an invalid model descriptor")
		}
		if descriptor.Protocol != "openai_chat_completions" {
			return nil, errors.New("OpenCode Zen allowlist contains an unsupported model protocol")
		}
		approved[id] = descriptor
	}
	return &OpenCodeZenProvider{
		chatEndpoint:   chatEndpoint,
		modelsEndpoint: modelsEndpoint,
		credential:     credential,
		client:         client,
		allowlist:      approved,
	}, nil
}

func NewSharedHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	return &http.Client{Transport: transport}
}

func (p *OpenCodeZenProvider) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if err := p.validateRequest(request); err != nil {
		return GenerateResult{}, err
	}
	response, err := p.sendChatRequest(ctx, request, false)
	if err != nil {
		return GenerateResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return GenerateResult{}, p.classifyHTTPError(response, request.Model)
	}

	var payload chatCompletionResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxProviderResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return GenerateResult{}, normalizedError(CodeModelResponseInvalid, request.Model, response.StatusCode, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return GenerateResult{}, normalizedError(CodeModelResponseInvalid, request.Model, response.StatusCode, err)
	}
	if len(payload.Choices) == 0 {
		return GenerateResult{}, normalizedError(CodeModelResponseInvalid, request.Model, response.StatusCode, errors.New("missing choices"))
	}
	choice := payload.Choices[0]
	model := payload.Model
	if model == "" {
		model = request.Model
	}
	return GenerateResult{
		ID:           payload.ID,
		Model:        model,
		Content:      choice.Message.Content,
		ToolCalls:    append([]ToolCall(nil), choice.Message.ToolCalls...),
		FinishReason: choice.FinishReason,
		Usage:        payload.Usage.normalized(),
	}, nil
}

func (p *OpenCodeZenProvider) Stream(ctx context.Context, request GenerateRequest) (<-chan StreamEvent, error) {
	if err := p.validateRequest(request); err != nil {
		return nil, err
	}
	response, err := p.sendChatRequest(ctx, request, true)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		return nil, p.classifyHTTPError(response, request.Model)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		response.Body.Close()
		return nil, normalizedError(CodeModelProtocol, request.Model, response.StatusCode, errors.New("unexpected content type"))
	}

	events := make(chan StreamEvent, 8)
	go p.readStream(ctx, response, request.Model, events)
	return events, nil
}

func (p *OpenCodeZenProvider) ListModels(ctx context.Context) ([]ModelDescriptor, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.modelsEndpoint, nil)
	if err != nil {
		return nil, normalizedError(CodeProviderUnavailable, "", 0, err)
	}
	p.authorize(request)
	response, err := p.client.Do(request)
	if err != nil {
		return nil, p.classifyTransportError(ctx, "", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, p.classifyHTTPError(response, "")
	}

	var payload modelListResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxProviderResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return nil, normalizedError(CodeModelResponseInvalid, "", response.StatusCode, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, normalizedError(CodeModelResponseInvalid, "", response.StatusCode, err)
	}

	models := make([]ModelDescriptor, 0, len(payload.Data))
	for _, upstream := range payload.Data {
		descriptor, ok := p.allowlist[upstream.ID]
		if !ok || !descriptor.Enabled {
			continue
		}
		descriptor.Object = upstream.Object
		descriptor.OwnedBy = upstream.OwnedBy
		models = append(models, descriptor)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func (p *OpenCodeZenProvider) validateRequest(request GenerateRequest) error {
	descriptor, ok := p.allowlist[request.Model]
	if !ok || !descriptor.Enabled {
		return normalizedError(CodeModelDisabled, request.Model, 0, nil)
	}
	if len(request.Messages) == 0 {
		return normalizedError(CodeModelProtocol, request.Model, 0, errors.New("messages are required"))
	}
	for _, message := range request.Messages {
		if strings.TrimSpace(message.Role) == "" || message.Content == "" {
			return normalizedError(CodeModelProtocol, request.Model, 0, errors.New("invalid message"))
		}
	}
	if request.MaxOutputTokens < 0 {
		return normalizedError(CodeModelProtocol, request.Model, 0, errors.New("negative output limit"))
	}
	return nil
}

func (p *OpenCodeZenProvider) sendChatRequest(ctx context.Context, request GenerateRequest, stream bool) (*http.Response, error) {
	payload := chatCompletionRequest{
		Model:       request.Model,
		Messages:    request.Messages,
		Stream:      stream,
		Tools:       request.Tools,
		ToolChoice:  request.ToolChoice,
		Temperature: request.Temperature,
		MaxTokens:   request.MaxOutputTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, normalizedError(CodeModelProtocol, request.Model, 0, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.chatEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, normalizedError(CodeModelProtocol, request.Model, 0, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if stream {
		httpRequest.Header.Set("Accept", "text/event-stream")
	} else {
		httpRequest.Header.Set("Accept", "application/json")
	}
	p.authorize(httpRequest)
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, p.classifyTransportError(ctx, request.Model, err)
	}
	return response, nil
}

func (p *OpenCodeZenProvider) authorize(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+p.credential.Reveal())
}

func (p *OpenCodeZenProvider) readStream(ctx context.Context, response *http.Response, model string, events chan<- StreamEvent) {
	defer close(events)
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), maxSSEEventBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			p.sendEvent(ctx, events, StreamEvent{Done: true})
			return
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			p.sendEvent(ctx, events, StreamEvent{Err: normalizedError(CodeModelResponseInvalid, model, response.StatusCode, err)})
			return
		}
		if chunk.Error != nil {
			p.sendEvent(ctx, events, StreamEvent{Err: classifyUpstreamError(http.StatusOK, model, *chunk.Error, 0)})
			return
		}
		if chunk.Usage != nil {
			usage := chunk.Usage.normalized()
			if !p.sendEvent(ctx, events, StreamEvent{Usage: &usage}) {
				return
			}
		}
		for _, choice := range chunk.Choices {
			event := StreamEvent{
				Delta:     choice.Delta.Content,
				ToolCalls: append([]ToolCall(nil), choice.Delta.ToolCalls...),
			}
			if choice.FinishReason != nil {
				event.FinishReason = *choice.FinishReason
			}
			if event.Delta == "" && len(event.ToolCalls) == 0 && event.FinishReason == "" {
				continue
			}
			if !p.sendEvent(ctx, events, event) {
				return
			}
		}
	}
	if ctx.Err() != nil {
		return
	}
	if err := scanner.Err(); err != nil {
		p.sendEvent(ctx, events, StreamEvent{Err: normalizedError(CodeModelProtocol, model, response.StatusCode, err)})
		return
	}
	p.sendEvent(ctx, events, StreamEvent{Err: normalizedError(CodeModelResponseInvalid, model, response.StatusCode, errors.New("stream ended without DONE"))})
}

func (p *OpenCodeZenProvider) sendEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (p *OpenCodeZenProvider) classifyTransportError(ctx context.Context, model string, err error) error {
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return normalizedError(CodeModelTimeout, model, 0, ctx.Err())
		}
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return normalizedError(CodeModelTimeout, model, 0, err)
	}
	return normalizedError(CodeProviderUnavailable, model, 0, err)
}

func (p *OpenCodeZenProvider) classifyHTTPError(response *http.Response, model string) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxProviderErrorBytes))
	var envelope upstreamErrorEnvelope
	_ = json.Unmarshal(body, &envelope)
	retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
	return classifyUpstreamError(response.StatusCode, model, envelope.Error, retryAfter)
}

func classifyUpstreamError(status int, model string, upstream upstreamError, retryAfter time.Duration) error {
	text := strings.ToLower(strings.Join([]string{upstream.Type, upstream.Code, upstream.Message}, " "))
	if status == http.StatusPaymentRequired || containsAny(text,
		"insufficient_quota",
		"insufficient quota",
		"provider exhausted",
		"balance exhausted",
		"credit balance",
		"billing hard limit",
	) {
		return &Error{Code: CodeProviderExhausted, Model: model, StatusCode: status, RetryAfter: retryAfter}
	}

	if containsAny(text, "rate_limit", "rate limit", "model busy") {
		return &Error{Code: CodeModelRateLimited, Model: model, StatusCode: status, RetryAfter: retryAfter}
	}
	if containsAny(text, "timeout", "timed out") {
		return &Error{Code: CodeModelTimeout, Model: model, StatusCode: status, RetryAfter: retryAfter}
	}

	code := CodeModelProtocol
	switch status {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		code = CodeModelTimeout
	case http.StatusTooManyRequests:
		code = CodeModelRateLimited
	case http.StatusNotFound, http.StatusGone:
		code = CodeModelUnavailable
	case http.StatusUnauthorized, http.StatusForbidden:
		code = CodeProviderUnavailable
	default:
		if status >= http.StatusInternalServerError {
			code = CodeProviderUnavailable
		}
	}
	return &Error{Code: code, Model: model, StatusCode: status, RetryAfter: retryAfter}
}

func normalizedError(code ErrorCode, model string, status int, cause error) error {
	return &Error{Code: code, Model: model, StatusCode: status, cause: cause}
}

func normalizeZenEndpoints(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", errors.New("OpenCode Zen endpoint must be an absolute URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", "", errors.New("OpenCode Zen endpoint uses an unsupported scheme")
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, openAIChatCompletionsSuffix) {
		return "", "", errors.New("OpenCode Zen endpoint must target chat/completions")
	}
	chat := parsed.String()
	parsed.Path = strings.TrimSuffix(parsed.Path, openAIChatCompletionsSuffix) + "/models"
	return chat, parsed.String(), nil
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

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if parsed, err := http.ParseTime(value); err == nil && parsed.After(now) {
		return parsed.Sub(now)
	}
	return 0
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

type chatCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Stream      bool             `json:"stream"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   upstreamUsage          `json:"usage"`
}

type chatCompletionChoice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type chatCompletionChunk struct {
	Choices []chatCompletionChunkChoice `json:"choices"`
	Usage   *upstreamUsage              `json:"usage,omitempty"`
	Error   *upstreamError              `json:"error,omitempty"`
}

type chatCompletionChunkChoice struct {
	Delta        Message `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type upstreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptDetails    struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u upstreamUsage) normalized() Usage {
	cached := u.CachedTokens
	if cached == 0 {
		cached = u.PromptDetails.CachedTokens
	}
	return Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
		CachedTokens: cached,
		TotalTokens:  u.TotalTokens,
	}
}

type modelListResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

type upstreamErrorEnvelope struct {
	Error upstreamError `json:"error"`
}

type upstreamError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

var _ ModelProvider = (*OpenCodeZenProvider)(nil)

func (p *OpenCodeZenProvider) String() string {
	return fmt.Sprintf("OpenCodeZenProvider(%s)", p.chatEndpoint)
}
