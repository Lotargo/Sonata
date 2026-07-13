package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"unicode"
)

const (
	headerOpenWebUIUserID    = "X-OpenWebUI-User-Id"
	headerOpenWebUIChatID    = "X-OpenWebUI-Chat-Id"
	headerOpenWebUIMessageID = "X-OpenWebUI-Message-Id"
	maxForwardedIDLength     = 256
)

type RequestIdentity struct {
	UserID    string
	ChatID    string
	MessageID string
}

type requestIdentityContextKey struct{}

func IdentityFromContext(ctx context.Context) (RequestIdentity, bool) {
	identity, ok := ctx.Value(requestIdentityContextKey{}).(RequestIdentity)
	return identity, ok
}

func requireInternalCredential(expected string) func(http.Handler) http.Handler {
	expectedDigest := sha256.Sum256([]byte(expected))
	configured := expected != ""

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !configured {
				writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "service authentication is not configured")
				return
			}

			provided, ok := bearerCredential(r.Header.Get("Authorization"))
			providedDigest := sha256.Sum256([]byte(provided))
			if !ok || subtle.ConstantTimeCompare(expectedDigest[:], providedDigest[:]) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeAPIError(w, http.StatusUnauthorized, "invalid_service_credential", "invalid service credential")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requireForwardedIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := RequestIdentity{
			UserID:    strings.TrimSpace(r.Header.Get(headerOpenWebUIUserID)),
			ChatID:    strings.TrimSpace(r.Header.Get(headerOpenWebUIChatID)),
			MessageID: strings.TrimSpace(r.Header.Get(headerOpenWebUIMessageID)),
		}
		if !validForwardedID(identity.UserID) || !validForwardedID(identity.ChatID) || !validForwardedID(identity.MessageID) {
			writeAPIError(w, http.StatusBadRequest, "invalid_forwarded_identity", "trusted user, chat and message IDs are required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIdentityContextKey{}, identity)))
	})
}

func bearerCredential(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func validForwardedID(value string) bool {
	if value == "" || len(value) > maxForwardedIDLength {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
