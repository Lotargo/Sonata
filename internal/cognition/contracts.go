package cognition

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Route string

const (
	RouteDirect Route = "direct"
	RouteFull   Route = "full"
)

func (route Route) Valid() bool {
	return route == RouteDirect || route == RouteFull
}

type Prism string

const (
	PrismEfficiency Prism = "efficiency"
	PrismCreativity Prism = "creativity"
	PrismPragmatism Prism = "pragmatism"
	PrismPhilosophy Prism = "philosophy"
	PrismEthics     Prism = "ethics"
)

var allPrisms = [...]Prism{
	PrismEfficiency,
	PrismCreativity,
	PrismPragmatism,
	PrismPhilosophy,
	PrismEthics,
}

func AllPrisms() []Prism {
	return append([]Prism(nil), allPrisms[:]...)
}

func (prism Prism) Valid() bool {
	switch prism {
	case PrismEfficiency, PrismCreativity, PrismPragmatism, PrismPhilosophy, PrismEthics:
		return true
	default:
		return false
	}
}

type Phase string

const (
	PhaseRouter           Phase = "router"
	PhaseRaw              Phase = "raw"
	PhaseCritical         Phase = "critical"
	PhaseSummary          Phase = "summary"
	PhaseSynthesisTooling Phase = "synthesis_tooling"
	PhaseSynthesisFinal   Phase = "synthesis_final"
)

type RuntimeRole string

const (
	RoleRouter RuntimeRole = "router"

	RoleEfficiencyRaw RuntimeRole = "efficiency_raw"
	RoleCreativityRaw RuntimeRole = "creativity_raw"
	RolePragmatismRaw RuntimeRole = "pragmatism_raw"
	RolePhilosophyRaw RuntimeRole = "philosophy_raw"
	RoleEthicsRaw     RuntimeRole = "ethics_raw"

	RoleEfficiencyCritical RuntimeRole = "efficiency_critical"
	RoleCreativityCritical RuntimeRole = "creativity_critical"
	RolePragmatismCritical RuntimeRole = "pragmatism_critical"
	RolePhilosophyCritical RuntimeRole = "philosophy_critical"
	RoleEthicsCritical     RuntimeRole = "ethics_critical"

	RoleEfficiencySummary RuntimeRole = "efficiency_summary"
	RoleCreativitySummary RuntimeRole = "creativity_summary"
	RolePragmatismSummary RuntimeRole = "pragmatism_summary"
	RolePhilosophySummary RuntimeRole = "philosophy_summary"
	RoleEthicsSummary     RuntimeRole = "ethics_summary"

	RoleSynthesisTooling RuntimeRole = "synthesis_tooling"
	RoleSynthesisFinal   RuntimeRole = "synthesis_final"
)

type RoleSpec struct {
	Role           RuntimeRole
	Phase          Phase
	Perspective    string
	InstructionID  string
	OutputContract string
	AllowsTools    bool
}

var runtimeRoleSpecs = [...]RoleSpec{
	{Role: RoleRouter, Phase: PhaseRouter, Perspective: "routing", InstructionID: "router", OutputContract: "router-v1"},
	{Role: RoleEfficiencyRaw, Phase: PhaseRaw, Perspective: string(PrismEfficiency), InstructionID: "prism.efficiency.raw", OutputContract: "prism-raw-v1"},
	{Role: RoleCreativityRaw, Phase: PhaseRaw, Perspective: string(PrismCreativity), InstructionID: "prism.creativity.raw", OutputContract: "prism-raw-v1"},
	{Role: RolePragmatismRaw, Phase: PhaseRaw, Perspective: string(PrismPragmatism), InstructionID: "prism.pragmatism.raw", OutputContract: "prism-raw-v1"},
	{Role: RolePhilosophyRaw, Phase: PhaseRaw, Perspective: string(PrismPhilosophy), InstructionID: "prism.philosophy.raw", OutputContract: "prism-raw-v1"},
	{Role: RoleEthicsRaw, Phase: PhaseRaw, Perspective: string(PrismEthics), InstructionID: "prism.ethics.raw", OutputContract: "prism-raw-v1"},
	{Role: RoleEfficiencyCritical, Phase: PhaseCritical, Perspective: string(PrismEfficiency), InstructionID: "prism.efficiency.critical", OutputContract: "prism-critical-v1"},
	{Role: RoleCreativityCritical, Phase: PhaseCritical, Perspective: string(PrismCreativity), InstructionID: "prism.creativity.critical", OutputContract: "prism-critical-v1"},
	{Role: RolePragmatismCritical, Phase: PhaseCritical, Perspective: string(PrismPragmatism), InstructionID: "prism.pragmatism.critical", OutputContract: "prism-critical-v1"},
	{Role: RolePhilosophyCritical, Phase: PhaseCritical, Perspective: string(PrismPhilosophy), InstructionID: "prism.philosophy.critical", OutputContract: "prism-critical-v1"},
	{Role: RoleEthicsCritical, Phase: PhaseCritical, Perspective: string(PrismEthics), InstructionID: "prism.ethics.critical", OutputContract: "prism-critical-v1"},
	{Role: RoleEfficiencySummary, Phase: PhaseSummary, Perspective: string(PrismEfficiency), InstructionID: "prism.efficiency.summary", OutputContract: "prism-summary-v1"},
	{Role: RoleCreativitySummary, Phase: PhaseSummary, Perspective: string(PrismCreativity), InstructionID: "prism.creativity.summary", OutputContract: "prism-summary-v1"},
	{Role: RolePragmatismSummary, Phase: PhaseSummary, Perspective: string(PrismPragmatism), InstructionID: "prism.pragmatism.summary", OutputContract: "prism-summary-v1"},
	{Role: RolePhilosophySummary, Phase: PhaseSummary, Perspective: string(PrismPhilosophy), InstructionID: "prism.philosophy.summary", OutputContract: "prism-summary-v1"},
	{Role: RoleEthicsSummary, Phase: PhaseSummary, Perspective: string(PrismEthics), InstructionID: "prism.ethics.summary", OutputContract: "prism-summary-v1"},
	{Role: RoleSynthesisTooling, Phase: PhaseSynthesisTooling, Perspective: "synthesis", InstructionID: "synthesis.tooling", OutputContract: "synthesis-tooling-v1", AllowsTools: true},
	{Role: RoleSynthesisFinal, Phase: PhaseSynthesisFinal, Perspective: "synthesis", InstructionID: "synthesis.final", OutputContract: "synthesis-final-v1"},
}

func RuntimeRoleSpecs() []RoleSpec {
	return append([]RoleSpec(nil), runtimeRoleSpecs[:]...)
}

type ConversationMessage struct {
	Role    string
	Content string
}

type ArtifactRef struct {
	ID      string
	Version int
	Hash    string
}

type ManifestRef struct {
	ArtifactRef
	Source string
}

type RoleStatus string

const (
	RoleStatusSucceeded RoleStatus = "SUCCEEDED"
	RoleStatusDegraded  RoleStatus = "DEGRADED"
	RoleStatusFailed    RoleStatus = "FAILED"
)

type RoleMetadata struct {
	Role        RuntimeRole
	Status      RoleStatus
	ModelID     string
	Latency     time.Duration
	Instruction ArtifactRef
	Manifest    ManifestRef
}

type ContextPack struct {
	Text        string
	CitationIDs []string
}

type EmotionReport struct {
	Text         string
	StateVersion int64
}

// RouterInput is intentionally minimal. Tools, RAG, EmotionReport and model
// selection are absent from this boundary by design.
type RouterInput struct {
	UserInput string
	History   []ConversationMessage
}

type RouterOutput struct {
	Route Route `json:"route"`
}

func (input RouterInput) Validate() error {
	if strings.TrimSpace(input.UserInput) == "" {
		return errors.New("router user input is required")
	}
	return nil
}

func (output RouterOutput) Validate() error {
	if !output.Route.Valid() {
		return fmt.Errorf("unsupported router route %q", output.Route)
	}
	return nil
}

type RawInput struct {
	Prism       Prism
	UserInput   string
	History     []ConversationMessage
	Context     ContextPack
	Emotion     EmotionReport
	Instruction ArtifactRef
	Manifest    ManifestRef
}

type PrismReport struct {
	Prism      Prism
	Content    string
	Confidence float64
	Metadata   RoleMetadata
}

type CriticalInput struct {
	Prism       Prism
	UserInput   string
	Context     ContextPack
	Emotion     EmotionReport
	Raw         PrismReport
	Instruction ArtifactRef
	Manifest    ManifestRef
}

type CriticalReport struct {
	Prism               Prism
	Content             string
	WeakAssumptions     []string
	UnprovenConclusions []string
	Confidence          float64
	Metadata            RoleMetadata
}

type SummaryInput struct {
	Prism       Prism
	Raw         PrismReport
	Critical    CriticalReport
	Instruction ArtifactRef
	Manifest    ManifestRef
}

type PrismSummary struct {
	Prism               Prism
	InitialPosition     string
	MainCritique        string
	RevisedPosition     string
	RejectedAssumptions []string
	OpenQuestions       []string
	Confidence          float64
	Metadata            RoleMetadata
}

type PrismDialogue struct {
	Raw      PrismReport
	Critical CriticalReport
	Summary  PrismSummary
}

type InternalDialogue struct {
	Branches map[Prism]PrismDialogue
}

func (dialogue InternalDialogue) ValidateFull() error {
	if len(dialogue.Branches) != len(allPrisms) {
		return fmt.Errorf("full dialogue requires %d prism branches", len(allPrisms))
	}
	for _, prism := range allPrisms {
		branch, exists := dialogue.Branches[prism]
		if !exists {
			return fmt.Errorf("full dialogue is missing %s prism", prism)
		}
		if branch.Raw.Prism != prism || branch.Critical.Prism != prism || branch.Summary.Prism != prism {
			return fmt.Errorf("full dialogue contains mismatched %s branch", prism)
		}
	}
	return nil
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	ErrorCode  string
}

type SynthesisToolingInput struct {
	UserInput   string
	History     []ConversationMessage
	Context     ContextPack
	Emotion     EmotionReport
	Dialogue    InternalDialogue
	Instruction ArtifactRef
	Manifest    ManifestRef
}

type SynthesisToolingOutput struct {
	PreliminaryDecision string
	ToolCalls           []ToolCall
	Metadata            RoleMetadata
}

type SynthesisFinalInput struct {
	Route               Route
	UserInput           string
	History             []ConversationMessage
	Context             ContextPack
	Emotion             EmotionReport
	Dialogue            InternalDialogue
	PreliminaryDecision string
	ToolResults         []ToolResult
	Instruction         ArtifactRef
	Manifest            ManifestRef
}

type SynthesisFinalOutput struct {
	Content  string
	Metadata RoleMetadata
}
