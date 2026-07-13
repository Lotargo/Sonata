package protected

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadRepositoryProtectedBundle(t *testing.T) {
	bundle, err := Load(os.DirFS(filepath.Join("..", "..", "protected")), "registry.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(bundle.Instructions) != 18 || len(bundle.DefaultManifests) != 18 {
		t.Fatalf("bundle sizes instructions=%d manifests=%d", len(bundle.Instructions), len(bundle.DefaultManifests))
	}
	ids := SortedInstructionIDs(bundle)
	if len(ids) != 18 || ids[0] != "prism.creativity.critical" || ids[len(ids)-1] != "synthesis.tooling" {
		t.Fatalf("unexpected sorted IDs: %#v", ids)
	}
	for id, instruction := range bundle.Instructions {
		if instruction.Identity != (Identity{Entity: "Sonata", Mode: "temporary-perspective", SeparateAgent: false}) {
			t.Fatalf("instruction %s has invalid identity: %#v", id, instruction.Identity)
		}
		if instruction.Metadata.Hash == "" || instruction.Metadata.Version != 1 {
			t.Fatalf("instruction %s has incomplete metadata: %#v", id, instruction.Metadata)
		}
		if id == "synthesis.tooling" {
			if instruction.Tools.Mode != "allowlist" || len(instruction.Tools.Allowed) != 2 {
				t.Fatalf("synthesis tooling policy = %#v", instruction.Tools)
			}
		} else if instruction.Tools.Mode != "none" || len(instruction.Tools.Allowed) != 0 {
			t.Fatalf("instruction %s unexpectedly has tools: %#v", id, instruction.Tools)
		}
	}
	for id, manifest := range bundle.DefaultManifests {
		if manifest.Metadata.Hash == "" || manifest.Target == "" || manifest.Guidance == "" {
			t.Fatalf("manifest %s is incomplete: %#v", id, manifest)
		}
	}
}

func TestLoadRejectsHashMismatch(t *testing.T) {
	files := repositoryMapFS(t)
	path := "instructions/router.xml"
	files[path] = &fstest.MapFile{Data: append([]byte(nil), files[path].Data...)}
	files[path].Data = append(files[path].Data, '\n')
	_, err := Load(files, "registry.json")
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
}

func TestLoadRejectsDTDAndUnknownXML(t *testing.T) {
	for name, test := range map[string]struct {
		mutate func(string) string
		want   string
	}{
		"DTD": {
			mutate: func(value string) string {
				return strings.Replace(value, "?>", "?>\n<!DOCTYPE sonata [<!ENTITY leak SYSTEM \"file:///etc/passwd\">]>", 1)
			},
			want: "directives",
		},
		"unknown attribute": {
			mutate: func(value string) string {
				return strings.Replace(value, "visibility=\"protected\"", "visibility=\"protected\" rogue=\"true\"", 1)
			},
			want: "unknown attribute",
		},
		"unknown element": {
			mutate: func(value string) string {
				return strings.Replace(value, "<purpose>", "<rogue />\n  <purpose>", 1)
			},
			want: "unknown element",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := repositoryMapFS(t)
			mutateArtifact(t, files, "instructions/router.xml", test.mutate)
			_, err := Load(files, "registry.json")
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestLoadRejectsIdentityOverride(t *testing.T) {
	files := repositoryMapFS(t)
	mutateArtifact(t, files, "instructions/router.xml", func(value string) string {
		return strings.Replace(value, "<separate-agent>false</separate-agent>", "<separate-agent>true</separate-agent>", 1)
	})
	_, err := Load(files, "registry.json")
	if err == nil || !strings.Contains(err.Error(), "single Sonata identity") {
		t.Fatalf("expected identity error, got %v", err)
	}
}

func TestLoadRejectsRegistryPathTraversal(t *testing.T) {
	files := repositoryMapFS(t)
	var registry registryDocument
	if err := json.Unmarshal(files["registry.json"].Data, &registry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	registry.Artifacts[0].Path = "../outside.xml"
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatalf("encode registry: %v", err)
	}
	files["registry.json"] = &fstest.MapFile{Data: data}
	_, err = Load(files, "registry.json")
	if err == nil || !strings.Contains(err.Error(), "invalid relative path") {
		t.Fatalf("expected path traversal error, got %v", err)
	}
}

func TestLoadRejectsManifestTargetWithoutInstruction(t *testing.T) {
	files := repositoryMapFS(t)
	manifestPath := "manifests/manifest.router.default.xml"
	mutateArtifact(t, files, manifestPath, func(value string) string {
		return strings.Replace(value, "target=\"router\"", "target=\"missing.instruction\"", 1)
	})
	_, err := Load(files, "registry.json")
	if err == nil || !strings.Contains(err.Error(), "targets unknown instruction") {
		t.Fatalf("expected target validation error, got %v", err)
	}
}

func repositoryMapFS(t *testing.T) fstest.MapFS {
	t.Helper()
	root := filepath.Join("..", "..", "protected")
	files := make(fstest.MapFS)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		t.Fatalf("copy protected artifacts: %v", err)
	}
	return files
}

func mutateArtifact(t *testing.T, files fstest.MapFS, artifactPath string, mutate func(string) string) {
	t.Helper()
	artifact, ok := files[artifactPath]
	if !ok {
		t.Fatalf("artifact %s not found", artifactPath)
	}
	updated := []byte(mutate(string(artifact.Data)))
	files[artifactPath] = &fstest.MapFile{Data: updated}

	var registry registryDocument
	if err := json.Unmarshal(files["registry.json"].Data, &registry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	hash := sha256.Sum256(updated)
	for index := range registry.Artifacts {
		if registry.Artifacts[index].Path == artifactPath {
			registry.Artifacts[index].SHA256 = hex.EncodeToString(hash[:])
			data, err := json.Marshal(registry)
			if err != nil {
				t.Fatalf("encode registry: %v", err)
			}
			files["registry.json"] = &fstest.MapFile{Data: data}
			return
		}
	}
	t.Fatalf("registry entry for %s not found", artifactPath)
}

func TestLoadRejectsDuplicateIdentity(t *testing.T) {
	files := repositoryMapFS(t)
	mutateArtifact(t, files, "instructions/router.xml", func(value string) string {
		identity := "  <identity>\n    <entity>Sonata</entity>\n    <mode>temporary-perspective</mode>\n    <separate-agent>false</separate-agent>\n  </identity>\n"
		return strings.Replace(value, identity, identity+identity, 1)
	})
	_, err := Load(files, "registry.json")
	if err == nil {
		t.Fatal("duplicate identity was accepted")
	}
}
