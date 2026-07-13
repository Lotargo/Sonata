package cognition

import (
	"errors"
	"fmt"
	"strings"
)

func requireRoleArtifacts(artifacts map[RuntimeRole]RoleArtifacts, role RuntimeRole) (RoleArtifacts, error) {
	value, exists := artifacts[role]
	if !exists {
		return RoleArtifacts{}, fmt.Errorf("artifacts for role %s are required", role)
	}
	if err := validateArtifactRef(value.Instruction, "role instruction"); err != nil {
		return RoleArtifacts{}, fmt.Errorf("role %s: %w", role, err)
	}
	if err := validateManifestRef(value.Manifest); err != nil {
		return RoleArtifacts{}, fmt.Errorf("role %s: %w", role, err)
	}
	return value, nil
}

func roleForPrismPhase(prism Prism, phase Phase) RuntimeRole {
	prefix := string(prism)
	switch phase {
	case PhaseRaw:
		return RuntimeRole(prefix + "_raw")
	case PhaseCritical:
		return RuntimeRole(prefix + "_critical")
	case PhaseSummary:
		return RuntimeRole(prefix + "_summary")
	default:
		return ""
	}
}

func validateRoleMetadataAgainstArtifacts(metadata RoleMetadata, role RuntimeRole, artifacts RoleArtifacts) error {
	if err := validateRoleMetadata(metadata, role); err != nil {
		return err
	}
	if metadata.Instruction != artifacts.Instruction {
		return errors.New("role instruction metadata does not match compiled input")
	}
	if metadata.Manifest != artifacts.Manifest {
		return errors.New("role manifest metadata does not match compiled input")
	}
	return nil
}

func validatePrismReport(report PrismReport, prism Prism, role RuntimeRole, artifacts RoleArtifacts) error {
	if report.Prism != prism {
		return fmt.Errorf("raw report prism is %q, want %q", report.Prism, prism)
	}
	if strings.TrimSpace(report.Content) == "" {
		return errors.New("raw report content is required")
	}
	if report.Confidence < 0 || report.Confidence > 1 {
		return errors.New("raw report confidence must be between 0 and 1")
	}
	return validateRoleMetadataAgainstArtifacts(report.Metadata, role, artifacts)
}

func validateCriticalReport(report CriticalReport, prism Prism, role RuntimeRole, artifacts RoleArtifacts) error {
	if report.Prism != prism {
		return fmt.Errorf("critical report prism is %q, want %q", report.Prism, prism)
	}
	if strings.TrimSpace(report.Content) == "" {
		return errors.New("critical report content is required")
	}
	if report.Confidence < 0 || report.Confidence > 1 {
		return errors.New("critical report confidence must be between 0 and 1")
	}
	return validateRoleMetadataAgainstArtifacts(report.Metadata, role, artifacts)
}

func validatePrismSummary(summary PrismSummary, prism Prism, role RuntimeRole, artifacts RoleArtifacts) error {
	if summary.Prism != prism {
		return fmt.Errorf("summary prism is %q, want %q", summary.Prism, prism)
	}
	if strings.TrimSpace(summary.RevisedPosition) == "" {
		return errors.New("summary revised position is required")
	}
	if summary.Confidence < 0 || summary.Confidence > 1 {
		return errors.New("summary confidence must be between 0 and 1")
	}
	return validateRoleMetadataAgainstArtifacts(summary.Metadata, role, artifacts)
}

func availablePrisms[T any](values map[Prism]T) []Prism {
	prisms := make([]Prism, 0, len(values))
	for _, prism := range allPrisms {
		if _, exists := values[prism]; exists {
			prisms = append(prisms, prism)
		}
	}
	return prisms
}

func cloneContextPack(value ContextPack) ContextPack {
	value.CitationIDs = append([]string(nil), value.CitationIDs...)
	return value
}

func clonePrismReport(value PrismReport) PrismReport {
	return value
}

func cloneCriticalReport(value CriticalReport) CriticalReport {
	value.WeakAssumptions = append([]string(nil), value.WeakAssumptions...)
	value.UnprovenConclusions = append([]string(nil), value.UnprovenConclusions...)
	return value
}

func clonePrismSummary(value PrismSummary) PrismSummary {
	value.RejectedAssumptions = append([]string(nil), value.RejectedAssumptions...)
	value.OpenQuestions = append([]string(nil), value.OpenQuestions...)
	return value
}

func cloneInternalDialogue(value InternalDialogue) InternalDialogue {
	cloned := InternalDialogue{Branches: make(map[Prism]PrismDialogue, len(value.Branches))}
	for prism, branch := range value.Branches {
		cloned.Branches[prism] = PrismDialogue{
			Raw:      clonePrismReport(branch.Raw),
			Critical: cloneCriticalReport(branch.Critical),
			Summary:  clonePrismSummary(branch.Summary),
		}
	}
	return cloned
}

func cloneToolCalls(values []ToolCall) []ToolCall {
	return append([]ToolCall(nil), values...)
}

func cloneToolResults(values []ToolResult) []ToolResult {
	return append([]ToolResult(nil), values...)
}

func cloneSynthesisToolingOutput(value SynthesisToolingOutput) SynthesisToolingOutput {
	value.ToolCalls = cloneToolCalls(value.ToolCalls)
	return value
}
