package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type secretResolverFunc func(context.Context, SecretRef) (SecretValue, error)

func (f secretResolverFunc) Resolve(ctx context.Context, ref SecretRef) (SecretValue, error) {
	return f(ctx, ref)
}

func TestSecretValueSerializationIsAlwaysRedacted(t *testing.T) {
	const rawSecret = "serialization-secret-7f91"
	value := struct {
		Secret SecretValue `json:"secret" yaml:"secret"`
	}{Secret: newSecretValue(rawSecret)}

	jsonData, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	yamlData, err := yaml.Marshal(value)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	formattedError := fmt.Errorf("startup failed: %v", value.Secret).Error()

	for format, output := range map[string]string{
		"json":  string(jsonData),
		"yaml":  string(yamlData),
		"error": formattedError,
	} {
		if strings.Contains(output, rawSecret) {
			t.Fatalf("%s output leaked raw secret: %q", format, output)
		}
		if !strings.Contains(output, "[REDACTED]") {
			t.Fatalf("%s output is not visibly redacted: %q", format, output)
		}
	}
}

func TestRuntimeSnapshotAndStartupErrorsDoNotLeakResolvedSecrets(t *testing.T) {
	const rawSecret = "resolved-secret-4c2a"
	resolver := secretResolverFunc(func(context.Context, SecretRef) (SecretValue, error) {
		return newSecretValue(rawSecret), nil
	})

	repositoryConfig := filepath.Join("..", "..", "config")
	cfg, err := NewLoader(resolver).Load(context.Background(), repositoryConfig, "local")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	yamlSnapshot, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal(RuntimeConfig) error = %v", err)
	}
	jsonSnapshot, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal(RuntimeConfig) error = %v", err)
	}
	for format, output := range map[string]string{
		"yaml": string(yamlSnapshot),
		"json": string(jsonSnapshot),
	} {
		if strings.Contains(output, rawSecret) {
			t.Fatalf("%s runtime snapshot leaked raw secret", format)
		}
	}

	invalidRoot := filepath.Join(t.TempDir(), "config")
	if err := os.CopyFS(invalidRoot, os.DirFS(repositoryConfig)); err != nil {
		t.Fatalf("os.CopyFS() error = %v", err)
	}
	invalidProfile := []byte("app:\n  environment: local\n  http_address: \"\"\n")
	profilePath := filepath.Join(invalidRoot, "environments", "local", "app.yaml")
	if err := os.WriteFile(profilePath, invalidProfile, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err = NewLoader(resolver).Load(context.Background(), invalidRoot, "local")
	if err == nil {
		t.Fatal("expected invalid configuration error")
	}
	if strings.Contains(err.Error(), rawSecret) {
		t.Fatalf("startup error leaked raw secret: %v", err)
	}
}
