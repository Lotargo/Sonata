package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepositoryConfigWithAnchors(t *testing.T) {
	for key, value := range map[string]string{
		"OPENCODE_ZEN_API_KEY":      "zen-secret",
		"DATABASE_URL":              "postgres://pool",
		"DATABASE_DIRECT_URL":       "postgres://direct",
		"LANGSEARCH_API_KEY":        "lang-secret",
		"QDRANT_URL":                "https://qdrant.example.test",
		"QDRANT_API_KEY":            "qdrant-secret",
		"OPENWEBUI_INTERNAL_SECRET": "internal-secret",
	} {
		t.Setenv(key, value)
	}

	root := filepath.Join("..", "..", "config")
	cfg, err := NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Models.Roles["raw"].Primary; got != "deepseek-v4-flash-free" {
		t.Fatalf("raw primary = %q", got)
	}
	if got := cfg.Models.Roles["critical"].Timeout.String(); got != "1m30s" {
		t.Fatalf("critical timeout = %q", got)
	}
	if got := cfg.Models.Roles["summary"].Primary; got != "nemotron-3-ultra-free" {
		t.Fatalf("summary primary = %q", got)
	}
	secret, ok := cfg.Secret("opencode_zen_master_key")
	if !ok || secret.Reveal() != "zen-secret" {
		t.Fatal("OpenCode Zen secret was not resolved")
	}
}

func TestDecodeStrictRejectsUnknownField(t *testing.T) {
	var cfg RuntimeConfig
	err := decodeStrict([]byte("app:\n  unexpected: true\n"), &cfg)
	if err == nil || !strings.Contains(err.Error(), "field unexpected") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDeepMergeReplacesSlicesAndRejectsTypeChange(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"items": []any{"one"}, "value": "base"}}
	src := map[string]any{"a": map[string]any{"items": []any{"two"}, "value": "override"}}
	if err := deepMerge(dst, src, "profile.yaml"); err != nil {
		t.Fatalf("deepMerge() error = %v", err)
	}
	items := dst["a"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0] != "two" {
		t.Fatalf("slice was not replaced: %#v", items)
	}

	bad := map[string]any{"a": "not-a-map"}
	if err := deepMerge(dst, bad, "bad.yaml"); err == nil {
		t.Fatal("expected type-change error")
	}
}

func TestMissingRequiredEnvironmentSecret(t *testing.T) {
	resolver := &DefaultSecretResolver{
		getenv:   func(string) (string, bool) { return "", false },
		readFile: func(string) ([]byte, error) { return nil, errors.New("not used") },
	}
	_, err := resolver.Resolve(context.Background(), SecretRef{Source: "env", Key: "MISSING", Required: true})
	if !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("expected ErrSecretUnavailable, got %v", err)
	}
}

func TestSecretValueAlwaysFormatsRedacted(t *testing.T) {
	secret := newSecretValue("do-not-leak")
	outputs := []string{
		fmt.Sprintf("%s", secret),
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%+v", secret),
		secret.LogValue().String(),
	}
	for _, output := range outputs {
		if strings.Contains(output, "do-not-leak") || output != "[REDACTED]" {
			t.Fatalf("secret formatting leaked or changed: %q", output)
		}
	}
	if got := secret.LogValue().Kind(); got != slog.KindString {
		t.Fatalf("unexpected slog kind %v", got)
	}
}
