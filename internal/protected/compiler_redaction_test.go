package protected

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestCompiledPromptRedactsSlogAndSerialization(t *testing.T) {
	bundle := testBundle()
	resolver, err := NewManifestResolver(bundle, DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := resolver.Resolve(ResolveManifestInput{InstructionID: "router", OwnerID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := NewPromptCompiler(bundle)
	if err != nil {
		t.Fatal(err)
	}
	const sensitive = "private-runtime-content-should-never-be-logged"
	compiled, err := compiler.Compile(CompileInput{
		InstructionID: "router",
		Manifest:      manifest,
		Runtime:       RuntimeContext{RoleInput: sensitive},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(compiled)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"[REDACTED COMPILED PROMPT]"` || strings.Contains(string(data), sensitive) {
		t.Fatalf("JSON=%s", data)
	}

	text, err := compiled.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != compiledPromptRedacted {
		t.Fatalf("text=%q", text)
	}

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logger.Info("compiled", "prompt", compiled)
	if !strings.Contains(output.String(), compiledPromptRedacted) || strings.Contains(output.String(), sensitive) {
		t.Fatalf("log=%s", output.String())
	}
}
