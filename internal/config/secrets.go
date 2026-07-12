package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

var ErrSecretUnavailable = errors.New("secret unavailable")

type SecretRef struct {
	Source   string `yaml:"source"`
	Key      string `yaml:"key,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Required bool   `yaml:"required"`
}

type SecretRegistry struct {
	Version int                  `yaml:"version"`
	Secrets map[string]SecretRef `yaml:"secrets"`
}

type SecretResolver interface {
	Resolve(ctx context.Context, ref SecretRef) (SecretValue, error)
}

type DefaultSecretResolver struct {
	getenv   func(string) (string, bool)
	readFile func(string) ([]byte, error)
}

func NewDefaultSecretResolver() *DefaultSecretResolver {
	return &DefaultSecretResolver{
		getenv:   os.LookupEnv,
		readFile: os.ReadFile,
	}
}

func (r *DefaultSecretResolver) Resolve(ctx context.Context, ref SecretRef) (SecretValue, error) {
	if err := ctx.Err(); err != nil {
		return SecretValue{}, err
	}

	switch ref.Source {
	case "env":
		if ref.Key == "" {
			return SecretValue{}, fmt.Errorf("env secret has empty key")
		}
		value, ok := r.getenv(ref.Key)
		if !ok || value == "" {
			return SecretValue{}, fmt.Errorf("%w: environment variable %s", ErrSecretUnavailable, ref.Key)
		}
		return newSecretValue(value), nil
	case "file":
		if ref.Path == "" {
			return SecretValue{}, fmt.Errorf("file secret has empty path")
		}
		data, err := r.readFile(ref.Path)
		if err != nil {
			return SecretValue{}, fmt.Errorf("%w: read %s: %v", ErrSecretUnavailable, ref.Path, err)
		}
		value := strings.TrimRight(string(data), "\r\n")
		if value == "" {
			return SecretValue{}, fmt.Errorf("%w: file %s is empty", ErrSecretUnavailable, ref.Path)
		}
		return newSecretValue(value), nil
	default:
		return SecretValue{}, fmt.Errorf("unsupported secret source %q", ref.Source)
	}
}

type SecretValue struct {
	value string
}

func newSecretValue(value string) SecretValue { return SecretValue{value: value} }

func (s SecretValue) Reveal() string { return s.value }
func (s SecretValue) Empty() bool    { return s.value == "" }
func (s SecretValue) String() string { return "[REDACTED]" }

func (s SecretValue) Format(state fmt.State, verb rune) {
	_, _ = io.WriteString(state, "[REDACTED]")
}

func (s SecretValue) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

func (s SecretValue) MarshalJSON() ([]byte, error) {
	return json.Marshal("[REDACTED]")
}

func (s SecretValue) MarshalYAML() (any, error) {
	return "[REDACTED]", nil
}
