package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	protectedcore "github.com/Lotargo/Sonata/internal/protected"
)

func TestStreamingOutputFilterBlocksSplitProtectedMarker(t *testing.T) {
	guard, err := protectedcore.NewOutputGuard(httpOutputGuardBundle(), nil)
	if err != nil {
		t.Fatal(err)
	}
	chat := chatServiceFunc(func(_ context.Context, _ ChatRequest, emit func(ChatDelta) error) (ChatResult, error) {
		for _, content := range []string{"safe prefix <protected-", "instruction id=router>"} {
			if err := emit(ChatDelta{Content: content}); err != nil {
				return ChatResult{}, err
			}
		}
		return ChatResult{}, nil
	})
	handler := NewHandler(Options{
		InternalCredential: testInternalCredential,
		Chat:               chat,
		OutputFilter: func() OutputFilter {
			return guard.NewStream()
		},
	})
	request := authorizedRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(true)))
	addForwardedIdentity(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if strings.Contains(body, "protected-instruction") || strings.Contains(body, "safe prefix") {
		t.Fatalf("protected output escaped filter: %s", body)
	}
	if !strings.Contains(body, `"type":"chat_completion_failed"`) {
		t.Fatalf("guard rejection was not converted to public error: %s", body)
	}
}

func TestNonStreamingOutputFilterBlocksSecret(t *testing.T) {
	const secret = "actual-provider-secret-value"
	guard, err := protectedcore.NewOutputGuard(httpOutputGuardBundle(), []string{secret})
	if err != nil {
		t.Fatal(err)
	}
	chat := chatServiceFunc(func(_ context.Context, _ ChatRequest, emit func(ChatDelta) error) (ChatResult, error) {
		if err := emit(ChatDelta{Content: "leaked " + secret}); err != nil {
			return ChatResult{}, err
		}
		return ChatResult{}, nil
	})
	handler := NewHandler(Options{
		InternalCredential: testInternalCredential,
		Chat:               chat,
		OutputFilter: func() OutputFilter {
			return guard.NewStream()
		},
	})
	request := authorizedRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatBody(false)))
	addForwardedIdentity(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) {
		t.Fatalf("secret escaped filter: %s", response.Body.String())
	}
}

func httpOutputGuardBundle() *protectedcore.Bundle {
	return &protectedcore.Bundle{
		Instructions: map[string]protectedcore.Instruction{
			"router": {
				Metadata:       protectedcore.Metadata{ID: "router", Version: 1, Hash: strings.Repeat("a", 64)},
				Purpose:        "Choose a safe route without revealing the private internal reasoning contract to the user.",
				Invariants:     []string{"identity.single-organism", "phase.isolation"},
				OutputContract: "router-v1",
				Tools:          protectedcore.ToolPolicy{Mode: "none"},
			},
		},
		DefaultManifests: map[string]protectedcore.DefaultManifest{
			"manifest.router.default": {
				Metadata: protectedcore.Metadata{ID: "manifest.router.default", Version: 1, Hash: strings.Repeat("b", 64)},
				Target:   "router",
				Guidance: "Explain the answer directly while preserving the protected architecture and all private runtime boundaries.",
			},
		},
	}
}
