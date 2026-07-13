package emotion

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lotargo/Sonata/internal/config"
)

func TestRepositoryAffectiveProfileLoadsInCanonicalOrder(t *testing.T) {
	setAffectiveTestSecrets(t)

	root := filepath.Join("..", "..", "config")
	runtimeConfig, err := config.NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatalf("load repository config: %v", err)
	}
	profile, err := NewAffectiveProfileFromConfig(runtimeConfig.Emotion)
	if err != nil {
		t.Fatalf("build affective profile: %v", err)
	}
	if profile.Version != "sonata-affective-v1.0.0" {
		t.Fatalf("profile version = %q", profile.Version)
	}
	if len(profile.Dynamics) != len(AllEmotions()) {
		t.Fatalf("emotion dynamics count = %d", len(profile.Dynamics))
	}
	for index, emotion := range AllEmotions() {
		if profile.Dynamics[index].Emotion != emotion {
			t.Fatalf("dynamics[%d] = %s, want %s", index, profile.Dynamics[index].Emotion, emotion)
		}
	}
	if len(profile.Drives) != 5 || profile.Drives[0].Kind != DriveCognition || profile.Drives[4].Kind != DriveSocialConnection {
		t.Fatalf("drive order = %#v", profile.Drives)
	}
	if len(profile.ComplexStates) != 5 || profile.ComplexStates[1].Kind != ComplexStateDepressive {
		t.Fatalf("complex state order = %#v", profile.ComplexStates)
	}

	depressive := profile.ComplexStates[1]
	if depressive.EntryThreshold <= depressive.ExitThreshold {
		t.Fatalf("depressive hysteresis is invalid: entry=%v exit=%v", depressive.EntryThreshold, depressive.ExitThreshold)
	}
	joyGain := findEmotionMultiplier(t, depressive.Effects.EmotionGain, Joy)
	joyCeiling := findEmotionMultiplier(t, depressive.Effects.EmotionCeiling, Joy)
	sadnessRecovery := findEmotionMultiplier(t, depressive.Effects.EmotionRecovery, Sadness)
	if joyGain != Multiplier(0.20) || joyCeiling != Multiplier(0.40) || sadnessRecovery != Multiplier(0.30) {
		t.Fatalf("depressive feedback differs from versioned config: gain=%v ceiling=%v sadness_recovery=%v", joyGain, joyCeiling, sadnessRecovery)
	}
}

func TestAffectiveProfileRejectsIncompleteEmotionMatrix(t *testing.T) {
	setAffectiveTestSecrets(t)

	root := filepath.Join("..", "..", "config")
	runtimeConfig, err := config.NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatal(err)
	}
	delete(runtimeConfig.Emotion.Affective.PersonalityInfluences, "joy")
	_, err = NewAffectiveProfileFromConfig(runtimeConfig.Emotion)
	if err == nil || !strings.Contains(err.Error(), "missing personality influence for joy") {
		t.Fatalf("expected missing matrix error, got %v", err)
	}
}

func TestConditionSignalParserRejectsUntypedPaths(t *testing.T) {
	t.Parallel()

	valid := []string{
		"emotion.sadness",
		"physiology.fatigue",
		"relationship.unresolved_hurt",
		"drive.social_connection.satisfaction",
	}
	for _, value := range valid {
		if _, err := ParseConditionSignal(value); err != nil {
			t.Fatalf("valid signal %q rejected: %v", value, err)
		}
	}
	invalid := []string{
		"emotion.unknown",
		"physiology.hunger",
		"drive.social_connection.unknown",
		"security.policy",
	}
	for _, value := range invalid {
		if _, err := ParseConditionSignal(value); err == nil {
			t.Fatalf("invalid signal %q accepted", value)
		}
	}
}

func findEmotionMultiplier(t *testing.T, values []EmotionMultiplier, target Emotion) Multiplier {
	t.Helper()
	for _, value := range values {
		if value.Emotion == target {
			return value.Value
		}
	}
	t.Fatalf("emotion multiplier %s not found", target)
	return 0
}

func setAffectiveTestSecrets(t *testing.T) {
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
