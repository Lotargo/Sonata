package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

type indexConfig struct {
	Version  int                 `yaml:"version"`
	Base     []string            `yaml:"base"`
	Profiles map[string][]string `yaml:"profiles"`
	Secrets  string              `yaml:"secrets"`
}

type Loader struct {
	resolver SecretResolver
}

func NewLoader(resolver SecretResolver) *Loader {
	if resolver == nil {
		resolver = NewDefaultSecretResolver()
	}
	return &Loader{resolver: resolver}
}

func (l *Loader) Load(ctx context.Context, root, profile string) (*RuntimeConfig, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("config root is required")
	}
	if strings.TrimSpace(profile) == "" {
		return nil, fmt.Errorf("config profile is required")
	}

	root = filepath.Clean(root)
	configFS := os.DirFS(root)

	var index indexConfig
	if err := readStrict(configFS, "index.yaml", &index); err != nil {
		return nil, fmt.Errorf("load config index: %w", err)
	}
	if index.Version != 1 {
		return nil, fmt.Errorf("unsupported config index version %d", index.Version)
	}
	profileFiles, ok := index.Profiles[profile]
	if !ok {
		return nil, fmt.Errorf("unknown config profile %q", profile)
	}
	if len(index.Base) == 0 {
		return nil, fmt.Errorf("config index contains no base fragments")
	}
	if index.Secrets == "" {
		return nil, fmt.Errorf("config index does not declare a secret registry")
	}

	merged := make(map[string]any)
	loaded := make([]string, 0, len(index.Base)+len(profileFiles)+2)
	loaded = append(loaded, "index.yaml")
	for _, name := range append(append([]string(nil), index.Base...), profileFiles...) {
		fragment, err := readMap(configFS, name)
		if err != nil {
			return nil, fmt.Errorf("load config fragment %s: %w", name, err)
		}
		if err := deepMerge(merged, fragment, name); err != nil {
			return nil, err
		}
		loaded = append(loaded, name)
	}

	encoded, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode merged config: %w", err)
	}
	var cfg RuntimeConfig
	if err := decodeStrict(encoded, &cfg); err != nil {
		return nil, fmt.Errorf("decode merged config: %w", err)
	}

	var registry SecretRegistry
	if err := readStrict(configFS, index.Secrets, &registry); err != nil {
		return nil, fmt.Errorf("load secret registry: %w", err)
	}
	if registry.Version != 1 {
		return nil, fmt.Errorf("unsupported secret registry version %d", registry.Version)
	}
	loaded = append(loaded, index.Secrets)

	resolved := make(map[string]SecretValue, len(registry.Secrets))
	for name, ref := range registry.Secrets {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("secret registry contains an empty logical ID")
		}
		value, err := l.resolver.Resolve(ctx, ref)
		if err != nil {
			if !ref.Required && errors.Is(err, ErrSecretUnavailable) {
				continue
			}
			return nil, fmt.Errorf("resolve secret %s: %w", name, err)
		}
		resolved[name] = value
	}

	cfg.profile = profile
	cfg.loadedFiles = loaded
	cfg.secrets = resolved
	if err := cfg.Validate(profile); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

func readStrict(fsys fs.FS, name string, out any) error {
	data, err := fs.ReadFile(fsys, filepath.ToSlash(name))
	if err != nil {
		return err
	}
	return decodeStrict(data, out)
}

func decodeStrict(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple YAML documents are not allowed")
		}
		return err
	}
	return nil
}

func readMap(fsys fs.FS, name string) (map[string]any, error) {
	data, err := fs.ReadFile(fsys, filepath.ToSlash(name))
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("fragment is empty")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents are not allowed")
		}
		return nil, err
	}
	return out, nil
}

func deepMerge(dst, src map[string]any, source string) error {
	for key, incoming := range src {
		current, exists := dst[key]
		if !exists {
			dst[key] = incoming
			continue
		}

		currentMap, currentIsMap := current.(map[string]any)
		incomingMap, incomingIsMap := incoming.(map[string]any)
		if currentIsMap && incomingIsMap {
			if err := deepMerge(currentMap, incomingMap, source); err != nil {
				return err
			}
			continue
		}
		if currentIsMap != incomingIsMap {
			return fmt.Errorf("type change for key %q while applying %s", key, source)
		}
		if !compatibleScalarTypes(current, incoming) {
			return fmt.Errorf("incompatible value types for key %q while applying %s: %T -> %T", key, source, current, incoming)
		}
		dst[key] = incoming
	}
	return nil
}

func compatibleScalarTypes(a, b any) bool {
	if a == nil || b == nil {
		return true
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta == tb {
		return true
	}
	return isNumberKind(ta.Kind()) && isNumberKind(tb.Kind())
}

func isNumberKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
