package protected

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	maxRegistryBytes = 1 << 20
	maxArtifactBytes = 256 << 10
)

var (
	stableIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._*-][a-z0-9]+)*$`)
	hashPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type registryDocument struct {
	Version   int             `json:"version"`
	Artifacts []registryEntry `json:"artifacts"`
}

type registryEntry struct {
	Kind    Kind   `json:"kind"`
	ID      string `json:"id"`
	Version int    `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

func Load(fsys fs.FS, registryPath string) (*Bundle, error) {
	if fsys == nil {
		return nil, errors.New("protected artifact filesystem is required")
	}
	if err := validateRelativePath(registryPath, ".json"); err != nil {
		return nil, fmt.Errorf("protected registry path: %w", err)
	}
	registryData, err := readBounded(fsys, registryPath, maxRegistryBytes)
	if err != nil {
		return nil, fmt.Errorf("read protected registry: %w", err)
	}
	var registry registryDocument
	decoder := json.NewDecoder(bytes.NewReader(registryData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("decode protected registry: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode protected registry: %w", err)
	}
	if registry.Version != 1 {
		return nil, fmt.Errorf("unsupported protected registry version %d", registry.Version)
	}
	if len(registry.Artifacts) == 0 {
		return nil, errors.New("protected registry contains no artifacts")
	}

	bundle := &Bundle{
		Instructions:     make(map[string]Instruction),
		DefaultManifests: make(map[string]DefaultManifest),
	}
	seenPaths := make(map[string]struct{}, len(registry.Artifacts))
	for _, entry := range registry.Artifacts {
		if err := validateRegistryEntry(entry); err != nil {
			return nil, err
		}
		if _, duplicate := seenPaths[entry.Path]; duplicate {
			return nil, fmt.Errorf("protected registry repeats path %q", entry.Path)
		}
		seenPaths[entry.Path] = struct{}{}
		data, err := readBounded(fsys, entry.Path, maxArtifactBytes)
		if err != nil {
			return nil, fmt.Errorf("read protected artifact %s: %w", entry.ID, err)
		}
		actualHash := sha256.Sum256(data)
		actualHashText := hex.EncodeToString(actualHash[:])
		if actualHashText != entry.SHA256 {
			return nil, fmt.Errorf("protected artifact %s hash mismatch", entry.ID)
		}
		switch entry.Kind {
		case KindInstruction:
			instruction, err := decodeInstruction(data, entry)
			if err != nil {
				return nil, fmt.Errorf("load protected instruction %s: %w", entry.ID, err)
			}
			if _, duplicate := bundle.Instructions[entry.ID]; duplicate {
				return nil, fmt.Errorf("duplicate protected instruction ID %q", entry.ID)
			}
			bundle.Instructions[entry.ID] = instruction
		case KindDefaultManifest:
			manifest, err := decodeDefaultManifest(data, entry)
			if err != nil {
				return nil, fmt.Errorf("load protected default manifest %s: %w", entry.ID, err)
			}
			if _, duplicate := bundle.DefaultManifests[entry.ID]; duplicate {
				return nil, fmt.Errorf("duplicate protected default manifest ID %q", entry.ID)
			}
			bundle.DefaultManifests[entry.ID] = manifest
		default:
			return nil, fmt.Errorf("unsupported protected artifact kind %q", entry.Kind)
		}
	}
	if err := validateBundle(bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

func validateRegistryEntry(entry registryEntry) error {
	if entry.Kind != KindInstruction && entry.Kind != KindDefaultManifest {
		return fmt.Errorf("protected artifact %q has unsupported kind %q", entry.ID, entry.Kind)
	}
	if !stableIDPattern.MatchString(entry.ID) {
		return fmt.Errorf("protected artifact has invalid ID %q", entry.ID)
	}
	if entry.Version < 1 {
		return fmt.Errorf("protected artifact %s has invalid version %d", entry.ID, entry.Version)
	}
	if err := validateRelativePath(entry.Path, ".xml"); err != nil {
		return fmt.Errorf("protected artifact %s path: %w", entry.ID, err)
	}
	if !hashPattern.MatchString(entry.SHA256) {
		return fmt.Errorf("protected artifact %s has invalid sha256", entry.ID)
	}
	return nil
}

func validateRelativePath(name, extension string) error {
	if !fs.ValidPath(name) || path.IsAbs(name) || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid relative path %q", name)
	}
	if clean := path.Clean(name); clean != name || name == "." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("unclean relative path %q", name)
	}
	if path.Ext(name) != extension {
		return fmt.Errorf("path %q must use %s", name, extension)
	}
	return nil
}

func readBounded(fsys fs.FS, name string, maximum int64) ([]byte, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes", maximum)
	}
	return data, nil
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

func validateBundle(bundle *Bundle) error {
	if len(bundle.Instructions) == 0 || len(bundle.DefaultManifests) == 0 {
		return errors.New("protected bundle requires instructions and default manifests")
	}
	targets := make(map[string]string, len(bundle.DefaultManifests))
	for manifestID, manifest := range bundle.DefaultManifests {
		if _, exists := bundle.Instructions[manifest.Target]; !exists {
			return fmt.Errorf("default manifest %s targets unknown instruction %s", manifestID, manifest.Target)
		}
		if previous, duplicate := targets[manifest.Target]; duplicate {
			return fmt.Errorf("default manifests %s and %s target the same instruction", previous, manifestID)
		}
		targets[manifest.Target] = manifestID
	}
	for instructionID := range bundle.Instructions {
		if _, exists := targets[instructionID]; !exists {
			return fmt.Errorf("instruction %s has no protected default manifest", instructionID)
		}
	}
	return nil
}

func SortedInstructionIDs(bundle *Bundle) []string {
	ids := make([]string, 0, len(bundle.Instructions))
	for id := range bundle.Instructions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
