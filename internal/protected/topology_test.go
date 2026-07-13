package protected_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/protected"
)

func TestRepositoryProtectedInstructionsMatchMiniMVPTopology(t *testing.T) {
	bundle, err := protected.Load(os.DirFS(filepath.Join("..", "..", "protected")), "registry.json")
	if err != nil {
		t.Fatal(err)
	}

	specs := cognition.RuntimeRoleSpecs()
	if len(bundle.Instructions) != len(specs) {
		t.Fatalf("protected instructions = %d, runtime roles = %d", len(bundle.Instructions), len(specs))
	}
	for _, spec := range specs {
		instruction, exists := bundle.Instructions[spec.InstructionID]
		if !exists {
			t.Fatalf("missing protected instruction %s", spec.InstructionID)
		}
		if instruction.Phase != string(spec.Phase) {
			t.Fatalf("instruction %s phase = %q, want %q", spec.InstructionID, instruction.Phase, spec.Phase)
		}
		if instruction.Perspective != spec.Perspective {
			t.Fatalf("instruction %s perspective = %q, want %q", spec.InstructionID, instruction.Perspective, spec.Perspective)
		}
		if instruction.OutputContract != spec.OutputContract {
			t.Fatalf("instruction %s output contract = %q, want %q", spec.InstructionID, instruction.OutputContract, spec.OutputContract)
		}
		if strings.TrimSpace(instruction.Purpose) == "" {
			t.Fatalf("instruction %s has no adapted purpose", spec.InstructionID)
		}
		if instruction.Identity != (protected.Identity{Entity: "Sonata", Mode: "temporary-perspective", SeparateAgent: false}) {
			t.Fatalf("instruction %s breaks the single Sonata identity", spec.InstructionID)
		}
		if spec.AllowsTools {
			if instruction.Tools.Mode != "allowlist" || len(instruction.Tools.Allowed) == 0 {
				t.Fatalf("instruction %s must use a non-empty tool allowlist", spec.InstructionID)
			}
		} else if instruction.Tools.Mode != "none" || len(instruction.Tools.Allowed) != 0 {
			t.Fatalf("instruction %s unexpectedly allows tools", spec.InstructionID)
		}
	}
}
