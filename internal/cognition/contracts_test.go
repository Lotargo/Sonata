package cognition

import (
	"reflect"
	"testing"
)

func TestRuntimeRoleTopologyContainsExactly18Calls(t *testing.T) {
	specs := RuntimeRoleSpecs()
	if len(specs) != 18 {
		t.Fatalf("runtime roles = %d, want 18", len(specs))
	}
	seen := make(map[RuntimeRole]struct{}, len(specs))
	for _, spec := range specs {
		if _, duplicate := seen[spec.Role]; duplicate {
			t.Fatalf("duplicate runtime role %q", spec.Role)
		}
		seen[spec.Role] = struct{}{}
		if spec.InstructionID == "" || spec.OutputContract == "" || spec.Perspective == "" {
			t.Fatalf("incomplete role spec: %#v", spec)
		}
		if spec.AllowsTools != (spec.Role == RoleSynthesisTooling) {
			t.Fatalf("role %s tool policy = %t", spec.Role, spec.AllowsTools)
		}
	}
}

func TestRouterInputBoundaryExcludesForbiddenCapabilities(t *testing.T) {
	typeOf := reflect.TypeOf(RouterInput{})
	if typeOf.NumField() != 2 {
		t.Fatalf("RouterInput fields = %d, want 2", typeOf.NumField())
	}
	for _, forbidden := range []string{"Tools", "RAG", "Context", "Emotion", "EmotionReport", "Model", "Models"} {
		if _, exists := typeOf.FieldByName(forbidden); exists {
			t.Fatalf("RouterInput exposes forbidden field %s", forbidden)
		}
	}
}

func TestInternalDialogueRequiresFiveIsolatedBranches(t *testing.T) {
	branches := make(map[Prism]PrismDialogue)
	for _, prism := range AllPrisms() {
		branches[prism] = PrismDialogue{
			Raw:      PrismReport{Prism: prism},
			Critical: CriticalReport{Prism: prism},
			Summary:  PrismSummary{Prism: prism},
		}
	}
	dialogue := InternalDialogue{Branches: branches}
	if err := dialogue.ValidateFull(); err != nil {
		t.Fatal(err)
	}
	delete(branches, PrismEthics)
	if err := dialogue.ValidateFull(); err == nil {
		t.Fatal("missing prism branch was accepted")
	}
}
