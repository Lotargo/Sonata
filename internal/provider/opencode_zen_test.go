package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

const testZenCredential = "zen-test-credential"

type testCredential string

func (c testCredential) Reveal() string { return string(c) }
func (c testCredential) Empty() bool    { return c == "" }

func TestOpenCodeZenGenerateUsesAllowlistAndCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testZenCredential {
			t.Fatalf("Authorization = %q", got)
		}
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "big-pickle" || request.Stream {
			t.Fatalf("unexpected request: %#v", request)
		}
		if request.MaxTokens != 512 || request.Temperature != 0.25 {
			t.Fatalf("generation options were not forwarded: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chat-1","model":"big-pickle","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14,"prompt_tokens_details":{"cached_tokens":2}}}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL+"/v1/chat/completions", defaultAllowlist())
	result, err := provider.Generate(context.Background(), GenerateRequest{
		Model:           "big-pickle",
		Messages:        []Message{{Role: "user", Content: "question"}},
		Temperature:     0.25,
		MaxOutputTokens: 512,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Content != "answer" || result.FinishReason != "stop" || result.Model != "big-pickle" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Usage != (Usage{InputTokens: 11, OutputTokens: 3, CachedTokens: 2, TotalTokens: 14}) {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
}

func TestOpenCodeZenProviderFromTypedConfig(t *testing.T) {
	provider, err := NewOpenCodeZenProviderFromConfig(
		config.EndpointConfig{
			Endpoint: "https://zen.example.invalid/v1/chat/completions",
			Protocol: "openai_chat_completions",
		},
		config.SecretValue{},
		&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("not called")
		})},
		map[string]config.ModelDefinition{
			"big-pickle": {
				ProviderRef: "open_code_zen", Protocol: "openai_chat_completions", Enabled: true, PrivacyClass: "standard",
			},
			"other-provider-model": {
				ProviderRef: "other", Protocol: "openai_chat_completions", Enabled: true,
			},
		},
	)
	if err == nil || provider != nil {
		t.Fatal("zero resolved SecretValue must fail closed")
	}
}

func TestOpenCodeZenGenerateToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Tools) != 1 || request.ToolChoice != "auto" || request.Tools[0].Function.Name != "web_search" {
			t.Fatalf("tool request was not forwarded: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chat-tool","model":"big-pickle","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"Sonata\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL+"/v1/chat/completions", defaultAllowlist())
	result, err := provider.Generate(context.Background(), GenerateRequest{
		Model:    "big-pickle",
		Messages: []Message{{Role: "user", Content: "search"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: FunctionDefinition{
				Name: "web_search", Parameters: map[string]any{"type": "object"},
			},
		}},
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.FinishReason != "tool_calls" || len(result.ToolCalls) != 1 || result.ToolCalls[0].Function.Name != "web_search" {
		t.Fatalf("unexpected tool call result: %#v", result)
	}
}

func TestOpenCodeZenStreamAndListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("test server does not support flushing")
			}
			for _, event := range []string{
				`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			} {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
				flusher.Flush()
			}
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"big-pickle","object":"model","owned_by":"zen"},{"id":"unknown-model","object":"model","owned_by":"zen"},{"id":"disabled-model","object":"model","owned_by":"zen"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	allowlist := defaultAllowlist()
	allowlist["disabled-model"] = ModelDescriptor{ID: "disabled-model", Protocol: "openai_chat_completions", Enabled: false}
	provider := newTestProvider(t, server.URL+"/v1/chat/completions", allowlist)
	events, err := provider.Stream(context.Background(), GenerateRequest{
		Model:    "big-pickle",
		Messages: []Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var content strings.Builder
	var finish string
	var usage *Usage
	var done bool
	for event := range events {
		if event.Err != nil {
			t.Fatalf("stream event error = %v", event.Err)
		}
		content.WriteString(event.Delta)
		if event.FinishReason != "" {
			finish = event.FinishReason
		}
		if event.Usage != nil {
			copy := *event.Usage
			usage = &copy
		}
		done = done || event.Done
	}
	if content.String() != "Hello world" || finish != "stop" || !done {
		t.Fatalf("unexpected stream content=%q finish=%q done=%v", content.String(), finish, done)
	}
	if usage == nil || *usage != (Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}) {
		t.Fatalf("unexpected stream usage: %#v", usage)
	}

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "big-pickle" || models[0].OwnedBy != "zen" {
		t.Fatalf("allowlist was not enforced: %#v", models)
	}
}

func TestOpenCodeZenRejectsDisabledModelBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL+"/v1/chat/completions", defaultAllowlist())
	_, err := provider.Generate(context.Background(), GenerateRequest{
		Model:    "north-mini-code-free",
		Messages: []Message{{Role: "user", Content: "question"}},
	})
	assertErrorCode(t, err, CodeModelDisabled)
	if calls.Load() != 0 {
		t.Fatalf("disabled model reached upstream %d times", calls.Load())
	}
}

func TestOpenCodeZenNormalizesAndRedactsUpstreamErrors(t *testing.T) {
	for name, test := range map[string]struct {
		status     int
		body       string
		want       ErrorCode
		fallback   bool
		retryAfter string
	}{
		"provider exhausted": {
			status: http.StatusPaymentRequired,
			body:   `{"error":{"message":"credit balance exhausted ` + testZenCredential + `","type":"insufficient_quota","code":"billing_hard_limit_reached"}}`,
			want:   CodeProviderExhausted,
		},
		"model rate limited": {
			status:     http.StatusTooManyRequests,
			body:       `{"error":{"message":"model busy","type":"rate_limit_error"}}`,
			want:       CodeModelRateLimited,
			fallback:   true,
			retryAfter: "3",
		},
		"model unavailable": {
			status:   http.StatusNotFound,
			body:     `{"error":{"message":"model missing"}}`,
			want:     CodeModelUnavailable,
			fallback: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.retryAfter != "" {
					w.Header().Set("Retry-After", test.retryAfter)
				}
				w.WriteHeader(test.status)
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			provider := newTestProvider(t, server.URL+"/v1/chat/completions", defaultAllowlist())
			_, err := provider.Generate(context.Background(), GenerateRequest{
				Model:    "big-pickle",
				Messages: []Message{{Role: "user", Content: "question"}},
			})
			assertErrorCode(t, err, test.want)
			if IsFallbackEligible(err) != test.fallback {
				t.Fatalf("fallback eligibility = %v, want %v", IsFallbackEligible(err), test.fallback)
			}
			if strings.Contains(err.Error(), testZenCredential) || strings.Contains(err.Error(), "credit balance") || strings.Contains(err.Error(), "model busy") {
				t.Fatalf("normalized error leaked upstream content: %v", err)
			}
			if test.retryAfter != "" {
				var providerErr *Error
				if !errors.As(err, &providerErr) || providerErr.RetryAfter != 3*time.Second {
					t.Fatalf("RetryAfter = %v", providerErr)
				}
			}
		})
	}
}

func TestOpenCodeZenRejectsInvalidSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL+"/v1/chat/completions", defaultAllowlist())
	_, err := provider.Generate(context.Background(), GenerateRequest{
		Model:    "big-pickle",
		Messages: []Message{{Role: "user", Content: "question"}},
	})
	assertErrorCode(t, err, CodeModelResponseInvalid)
}

func TestOpenCodeZenRequestCancellation(t *testing.T) {
	started := make(chan struct{})
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	provider, err := newOpenCodeZenProvider(
		"https://zen.example.invalid/v1/chat/completions",
		testCredential(testZenCredential),
		client,
		defaultAllowlist(),
	)
	if err != nil {
		t.Fatalf("newOpenCodeZenProvider() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, generateErr := provider.Generate(ctx, GenerateRequest{
			Model:    "big-pickle",
			Messages: []Message{{Role: "user", Content: "question"}},
		})
		result <- generateErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()
	select {
	case generateErr := <-result:
		if !errors.Is(generateErr, context.Canceled) {
			t.Fatalf("Generate() error = %v, want context.Canceled", generateErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Generate() did not return after cancellation")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newTestProvider(t *testing.T, endpoint string, allowlist map[string]ModelDescriptor) *OpenCodeZenProvider {
	t.Helper()
	provider, err := newOpenCodeZenProvider(endpoint, testCredential(testZenCredential), &http.Client{Timeout: time.Second}, allowlist)
	if err != nil {
		t.Fatalf("newOpenCodeZenProvider() error = %v", err)
	}
	return provider
}

func defaultAllowlist() map[string]ModelDescriptor {
	return map[string]ModelDescriptor{
		"big-pickle": {
			ID: "big-pickle", Protocol: "openai_chat_completions", PrivacyClass: "standard", Enabled: true,
		},
		"mimo-v2.5-free": {
			ID: "mimo-v2.5-free", Protocol: "openai_chat_completions", PrivacyClass: "standard", Enabled: true,
		},
	}
}

func assertErrorCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s", want)
	}
	got, ok := ErrorCodeOf(err)
	if !ok || got != want {
		t.Fatalf("error code = %q, want %q; err=%v", got, want, err)
	}
}
