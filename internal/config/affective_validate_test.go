package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryAffectiveConfigPassesSemanticValidation(t *testing.T) {
	setRepositoryConfigSecrets(t)

	root := filepath.Join("..", "..", "config")
	cfg, err := NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatalf("load repository config: %v", err)
	}
	if problems := cfg.Emotion.ValidateAffective(); len(problems) != 0 {
		t.Fatalf("valid affective config returned problems: %v", problems)
	}
	if cfg.Emotion.Affective.ProfileVersion != "sonata-affective-v1.0.0" {
		t.Fatalf("profile version = %q", cfg.Emotion.Affective.ProfileVersion)
	}
}

func TestAffectiveValidationRejectsIncompleteMatricesAndMissingHysteresis(t *testing.T) {
	setRepositoryConfigSecrets(t)

	root := filepath.Join("..", "..", "config")
	cfg, err := NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Emotion.Affective.PhysiologyInfluences, "joy")
	depressive := cfg.Emotion.Affective.ComplexStates["depressive"]
	depressive.ExitThreshold = depressive.EntryThreshold
	cfg.Emotion.Affective.ComplexStates["depressive"] = depressive

	problems := strings.Join(cfg.Emotion.ValidateAffective(), "; ")
	if !strings.Contains(problems, "physiology_influences is missing joy") {
		t.Fatalf("missing matrix was not rejected: %s", problems)
	}
	if !strings.Contains(problems, "entry_threshold must exceed exit_threshold") {
		t.Fatalf("missing hysteresis was not rejected: %s", problems)
	}
}

func setRepositoryConfigSecrets(t *testing.T) {
	t.Helper()
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
}
