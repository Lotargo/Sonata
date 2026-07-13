package protected

import (
	"errors"
	"strings"
	"testing"
)

func TestOutputGuardRejectsProtectedFragmentsAndMarkers(t *testing.T) {
	guard, err := NewOutputGuard(outputGuardBundle(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"prefix <protected-instruction id=router>",
		"The protected purpose says: Choose a safe route without revealing the private internal reasoning contract to the user.",
		"identity.single-organism",
	} {
		if err := guard.Check(value); !errors.Is(err, ErrOutputRejected) {
			t.Fatalf("Check(%q) error=%v", value, err)
		}
	}
}

func TestOutputGuardRejectsSecretPatterns(t *testing.T) {
	guard, err := NewOutputGuard(outputGuardBundle(), []string{"actual-secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"actual-secret-value",
		"Authorization: Bearer abcdefghijklmnop",
		"postgres://user:password@database.internal/sonata",
		"sk-abcdefghijklmnopqrstuvwxyz",
	} {
		if err := guard.Check(value); !errors.Is(err, ErrOutputRejected) {
			t.Fatalf("Check(%q) error=%v", value, err)
		}
	}
}

func TestOutputGuardStreamDetectsSplitLeakBeforeRelease(t *testing.T) {
	guard, err := NewOutputGuard(outputGuardBundle(), nil)
	if err != nil {
		t.Fatal(err)
	}
	stream := guard.NewStream()
	first, err := stream.Push("safe introduction then <protected-")
	if err != nil {
		t.Fatal(err)
	}
	if first != "" {
		t.Fatalf("first=%q", first)
	}
	second, err := stream.Push("instruction id=router>")
	if !errors.Is(err, ErrOutputRejected) {
		t.Fatalf("error=%v", err)
	}
	if second != "" {
		t.Fatalf("second=%q", second)
	}
	if tail, err := stream.Close(); !errors.Is(err, ErrOutputRejected) || tail != "" {
		t.Fatalf("Close()=(%q,%v)", tail, err)
	}
}

func TestOutputGuardStreamReleasesSafeUTF8(t *testing.T) {
	guard, err := NewOutputGuard(outputGuardBundle(), nil)
	if err != nil {
		t.Fatal(err)
	}
	stream := guard.NewStream()
	input := strings.Repeat("Безопасный ответ. ", 40)
	var output strings.Builder
	for _, part := range []string{input[:len(input)/2], input[len(input)/2:]} {
		safe, err := stream.Push(part)
		if err != nil {
			t.Fatal(err)
		}
		output.WriteString(safe)
	}
	tail, err := stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	output.WriteString(tail)
	if output.String() != input {
		t.Fatal("guard changed safe UTF-8 output")
	}
}

func outputGuardBundle() *Bundle {
	return &Bundle{
		Instructions: map[string]Instruction{
			"router": {
				Metadata:       Metadata{ID: "router", Version: 1, Hash: strings.Repeat("a", 64)},
				Purpose:        "Choose a safe route without revealing the private internal reasoning contract to the user.",
				Invariants:     []string{"identity.single-organism", "phase.isolation"},
				OutputContract: "router-v1",
				Tools:          ToolPolicy{Mode: "none"},
			},
		},
		DefaultManifests: map[string]DefaultManifest{
			"manifest.router.default": {
				Metadata: Metadata{ID: "manifest.router.default", Version: 1, Hash: strings.Repeat("b", 64)},
				Target:   "router",
				Guidance: "Explain the answer directly while preserving the protected architecture and all private runtime boundaries.",
			},
		},
	}
}
