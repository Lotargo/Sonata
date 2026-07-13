package emotion

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type EmotionDynamics struct {
	Emotion          Emotion
	Baseline         Unit
	ExcitationGain   NonNegative
	RecoveryRate     NonNegative
	Persistence      Unit
	Ceiling          Unit
	MaxPositiveDelta Unit
	MaxNegativeDelta Unit
}

func (dynamics EmotionDynamics) Validate() error {
	if !dynamics.Emotion.Valid() {
		return fmt.Errorf("invalid dynamics emotion %q", dynamics.Emotion)
	}
	fields := [...]struct {
		name string
		err  error
	}{
		{name: "baseline", err: dynamics.Baseline.Validate()},
		{name: "excitation_gain", err: dynamics.ExcitationGain.Validate()},
		{name: "recovery_rate", err: dynamics.RecoveryRate.Validate()},
		{name: "persistence", err: dynamics.Persistence.Validate()},
		{name: "ceiling", err: dynamics.Ceiling.Validate()},
		{name: "max_positive_delta", err: dynamics.MaxPositiveDelta.Validate()},
		{name: "max_negative_delta", err: dynamics.MaxNegativeDelta.Validate()},
	}
	for _, field := range fields {
		if field.err != nil {
			return fmt.Errorf("emotion %s %s: %w", dynamics.Emotion, field.name, field.err)
		}
	}
	if dynamics.Ceiling < dynamics.Baseline {
		return fmt.Errorf("emotion %s ceiling cannot be below baseline", dynamics.Emotion)
	}
	if dynamics.MaxPositiveDelta == 0 || dynamics.MaxNegativeDelta == 0 {
		return fmt.Errorf("emotion %s deltas must be positive", dynamics.Emotion)
	}
	return nil
}

type PersonalityInfluence struct {
	Emotion           Emotion
	Openness          SignedUnit
	Conscientiousness SignedUnit
	Extraversion      SignedUnit
	Agreeableness     SignedUnit
	Neuroticism       SignedUnit
}

func (influence PersonalityInfluence) Validate() error {
	if !influence.Emotion.Valid() {
		return fmt.Errorf("invalid personality influence emotion %q", influence.Emotion)
	}
	values := [...]SignedUnit{
		influence.Openness,
		influence.Conscientiousness,
		influence.Extraversion,
		influence.Agreeableness,
		influence.Neuroticism,
	}
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("emotion %s personality influence: %w", influence.Emotion, err)
		}
	}
	return nil
}

type PhysiologyInfluence struct {
	Emotion    Emotion
	Fatigue    SignedUnit
	Arousal    SignedUnit
	Energy     SignedUnit
	StressLoad SignedUnit
	Stability  SignedUnit
}

func (influence PhysiologyInfluence) Validate() error {
	if !influence.Emotion.Valid() {
		return fmt.Errorf("invalid physiology influence emotion %q", influence.Emotion)
	}
	values := [...]SignedUnit{
		influence.Fatigue,
		influence.Arousal,
		influence.Energy,
		influence.StressLoad,
		influence.Stability,
	}
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("emotion %s physiology influence: %w", influence.Emotion, err)
		}
	}
	return nil
}

type InteractionMode string

const (
	InteractionExcite  InteractionMode = "excite"
	InteractionInhibit InteractionMode = "inhibit"
)

func (mode InteractionMode) Valid() bool {
	return mode == InteractionExcite || mode == InteractionInhibit
}

type Interaction struct {
	From   Emotion
	To     Emotion
	Mode   InteractionMode
	Weight SignedUnit
}

func (interaction Interaction) Key() string {
	return string(interaction.From) + "|" + string(interaction.To) + "|" + string(interaction.Mode)
}

func (interaction Interaction) Validate() error {
	if !interaction.From.Valid() || !interaction.To.Valid() {
		return errors.New("interaction emotions must be valid")
	}
	if interaction.From == interaction.To {
		return errors.New("interaction cannot target itself")
	}
	if !interaction.Mode.Valid() {
		return fmt.Errorf("invalid interaction mode %q", interaction.Mode)
	}
	if err := interaction.Weight.Validate(); err != nil {
		return err
	}
	if interaction.Mode == InteractionExcite && interaction.Weight <= 0 {
		return errors.New("excitation weight must be positive")
	}
	if interaction.Mode == InteractionInhibit && interaction.Weight >= 0 {
		return errors.New("inhibition weight must be negative")
	}
	return nil
}

type StimulusDriveEffect struct {
	Stimulus StimulusKind
	Weight   SignedUnit
}

type EmotionWeight struct {
	Emotion Emotion
	Weight  SignedUnit
}

type DriveDefinition struct {
	Kind           DriveKind
	DefinitionID   string
	Baseline       Unit
	GrowthRate     NonNegative
	Satisfaction   []StimulusDriveEffect
	EmotionEffects []EmotionWeight
}

func (definition DriveDefinition) Validate() error {
	if !definition.Kind.Valid() {
		return fmt.Errorf("invalid drive kind %q", definition.Kind)
	}
	if strings.TrimSpace(definition.DefinitionID) == "" {
		return errors.New("drive definition ID is required")
	}
	if err := definition.Baseline.Validate(); err != nil {
		return err
	}
	if err := definition.GrowthRate.Validate(); err != nil {
		return err
	}
	last := ""
	for _, effect := range definition.Satisfaction {
		if !effect.Stimulus.Valid() {
			return fmt.Errorf("invalid stimulus %q", effect.Stimulus)
		}
		if err := effect.Weight.Validate(); err != nil {
			return err
		}
		key := string(effect.Stimulus)
		if key <= last {
			return errors.New("drive satisfaction effects must be sorted and unique")
		}
		last = key
	}
	last = ""
	for _, effect := range definition.EmotionEffects {
		if !effect.Emotion.Valid() {
			return fmt.Errorf("invalid emotion %q", effect.Emotion)
		}
		if err := effect.Weight.Validate(); err != nil {
			return err
		}
		key := string(effect.Emotion)
		if key <= last {
			return errors.New("drive emotion effects must be sorted and unique")
		}
		last = key
	}
	return nil
}

type SignalLayer string

const (
	SignalEmotion      SignalLayer = "emotion"
	SignalPhysiology   SignalLayer = "physiology"
	SignalRelationship SignalLayer = "relationship"
	SignalDrive        SignalLayer = "drive"
)

type ConditionSignal struct {
	Layer SignalLayer
	Name  string
	Field string
}

func ParseConditionSignal(raw string) (ConditionSignal, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) < 2 {
		return ConditionSignal{}, fmt.Errorf("invalid condition signal %q", raw)
	}
	signal := ConditionSignal{Layer: SignalLayer(parts[0]), Name: parts[1]}
	switch signal.Layer {
	case SignalEmotion:
		if len(parts) != 2 || !Emotion(signal.Name).Valid() {
			return ConditionSignal{}, fmt.Errorf("invalid emotion signal %q", raw)
		}
	case SignalPhysiology:
		if len(parts) != 2 || !validPhysiologyField(signal.Name) {
			return ConditionSignal{}, fmt.Errorf("invalid physiology signal %q", raw)
		}
	case SignalRelationship:
		if len(parts) != 2 || !validRelationshipField(signal.Name) {
			return ConditionSignal{}, fmt.Errorf("invalid relationship signal %q", raw)
		}
	case SignalDrive:
		if len(parts) != 3 || !DriveKind(signal.Name).Valid() || !oneOf(parts[2], "level", "satisfaction", "urgency") {
			return ConditionSignal{}, fmt.Errorf("invalid drive signal %q", raw)
		}
		signal.Field = parts[2]
	default:
		return ConditionSignal{}, fmt.Errorf("invalid condition signal layer %q", signal.Layer)
	}
	return signal, nil
}

func (signal ConditionSignal) String() string {
	if signal.Field != "" {
		return string(signal.Layer) + "." + signal.Name + "." + signal.Field
	}
	return string(signal.Layer) + "." + signal.Name
}

type ConditionOperator string

const (
	ConditionGTE ConditionOperator = ">="
	ConditionLTE ConditionOperator = "<="
)

func (operator ConditionOperator) Valid() bool {
	return operator == ConditionGTE || operator == ConditionLTE
}

type Condition struct {
	Signal    ConditionSignal
	Operator  ConditionOperator
	Threshold Unit
	Weight    Unit
}

func (condition Condition) Key() string {
	return condition.Signal.String() + "|" + string(condition.Operator)
}

func (condition Condition) Validate() error {
	if !condition.Operator.Valid() {
		return fmt.Errorf("invalid condition operator %q", condition.Operator)
	}
	if _, err := ParseConditionSignal(condition.Signal.String()); err != nil {
		return err
	}
	if err := condition.Threshold.Validate(); err != nil {
		return err
	}
	if err := condition.Weight.Validate(); err != nil {
		return err
	}
	if condition.Weight == 0 {
		return errors.New("condition weight must be positive")
	}
	return nil
}

type EmotionMultiplier struct {
	Emotion Emotion
	Value   Multiplier
}

type EmotionShift struct {
	Emotion Emotion
	Value   SignedUnit
}

type NamedMultiplier struct {
	Name  string
	Value Multiplier
}

type DriveMultiplier struct {
	Drive DriveKind
	Value Multiplier
}

type StateEffects struct {
	EmotionGain          []EmotionMultiplier
	EmotionRecovery      []EmotionMultiplier
	EmotionPersistence   []EmotionMultiplier
	EmotionCeiling       []EmotionMultiplier
	EmotionInhibition    []EmotionMultiplier
	EmotionTargetShifts  []EmotionShift
	PhysiologyRecovery   []NamedMultiplier
	RelationshipRecovery []NamedMultiplier
	DriveUrgency         []DriveMultiplier
	ReportBiases         []string
}

func (effects StateEffects) Validate() error {
	for _, group := range [][]EmotionMultiplier{
		effects.EmotionGain,
		effects.EmotionRecovery,
		effects.EmotionPersistence,
		effects.EmotionCeiling,
		effects.EmotionInhibition,
	} {
		last := ""
		for _, modifier := range group {
			if !modifier.Emotion.Valid() {
				return fmt.Errorf("invalid effect emotion %q", modifier.Emotion)
			}
			if err := modifier.Value.Validate(); err != nil {
				return err
			}
			key := string(modifier.Emotion)
			if key <= last {
				return errors.New("emotion modifiers must be sorted and unique")
			}
			last = key
		}
	}
	last := ""
	for _, shift := range effects.EmotionTargetShifts {
		if !shift.Emotion.Valid() {
			return fmt.Errorf("invalid target shift emotion %q", shift.Emotion)
		}
		if err := shift.Value.Validate(); err != nil {
			return err
		}
		key := string(shift.Emotion)
		if key <= last {
			return errors.New("emotion shifts must be sorted and unique")
		}
		last = key
	}
	last = ""
	for _, modifier := range effects.PhysiologyRecovery {
		if !validPhysiologyField(modifier.Name) {
			return fmt.Errorf("invalid physiology recovery field %q", modifier.Name)
		}
		if err := modifier.Value.Validate(); err != nil {
			return err
		}
		if modifier.Name <= last {
			return errors.New("physiology modifiers must be sorted and unique")
		}
		last = modifier.Name
	}
	last = ""
	for _, modifier := range effects.RelationshipRecovery {
		if !validRelationshipField(modifier.Name) {
			return fmt.Errorf("invalid relationship recovery field %q", modifier.Name)
		}
		if err := modifier.Value.Validate(); err != nil {
			return err
		}
		if modifier.Name <= last {
			return errors.New("relationship modifiers must be sorted and unique")
		}
		last = modifier.Name
	}
	last = ""
	for _, modifier := range effects.DriveUrgency {
		if !modifier.Drive.Valid() {
			return fmt.Errorf("invalid drive modifier %q", modifier.Drive)
		}
		if err := modifier.Value.Validate(); err != nil {
			return err
		}
		key := string(modifier.Drive)
		if key <= last {
			return errors.New("drive modifiers must be sorted and unique")
		}
		last = key
	}
	if !sort.StringsAreSorted(effects.ReportBiases) {
		return errors.New("report biases must be sorted")
	}
	seen := make(map[string]struct{}, len(effects.ReportBiases))
	for _, bias := range effects.ReportBiases {
		if strings.TrimSpace(bias) == "" {
			return errors.New("report bias cannot be empty")
		}
		if _, exists := seen[bias]; exists {
			return errors.New("report biases must be unique")
		}
		seen[bias] = struct{}{}
	}
	return nil
}

type ComplexStateDefinition struct {
	Kind             ComplexStateKind
	DefinitionID     string
	EntryConditions  []Condition
	ExitConditions   []Condition
	MinEntryDuration time.Duration
	MinExitDuration  time.Duration
	EntryThreshold   Unit
	ExitThreshold    Unit
	Effects          StateEffects
}

func (definition ComplexStateDefinition) Validate() error {
	if !definition.Kind.Valid() {
		return fmt.Errorf("invalid complex state kind %q", definition.Kind)
	}
	if strings.TrimSpace(definition.DefinitionID) == "" {
		return errors.New("complex state definition ID is required")
	}
	if err := validateConditions(definition.EntryConditions); err != nil {
		return fmt.Errorf("entry conditions: %w", err)
	}
	if err := validateConditions(definition.ExitConditions); err != nil {
		return fmt.Errorf("exit conditions: %w", err)
	}
	if definition.MinEntryDuration <= 0 || definition.MinExitDuration <= 0 {
		return errors.New("complex state durations must be positive")
	}
	if err := definition.EntryThreshold.Validate(); err != nil {
		return err
	}
	if err := definition.ExitThreshold.Validate(); err != nil {
		return err
	}
	if definition.EntryThreshold <= definition.ExitThreshold {
		return errors.New("entry threshold must exceed exit threshold for hysteresis")
	}
	return definition.Effects.Validate()
}

func validateConditions(conditions []Condition) error {
	if len(conditions) == 0 {
		return errors.New("conditions cannot be empty")
	}
	last := ""
	for _, condition := range conditions {
		if err := condition.Validate(); err != nil {
			return err
		}
		key := condition.Key()
		if key <= last {
			return errors.New("conditions must be sorted and unique")
		}
		last = key
	}
	return nil
}

type AffectiveProfile struct {
	Version               string
	IntegrationStep       time.Duration
	MaxSubsteps           int
	Personality           Personality
	Dynamics              []EmotionDynamics
	PersonalityInfluences []PersonalityInfluence
	PhysiologyInfluences  []PhysiologyInfluence
	Interactions          []Interaction
	Drives                []DriveDefinition
	ComplexStates         []ComplexStateDefinition
}

func (profile AffectiveProfile) Validate() error {
	if strings.TrimSpace(profile.Version) == "" {
		return errors.New("profile version is required")
	}
	if profile.IntegrationStep <= 0 {
		return errors.New("integration step must be positive")
	}
	if profile.MaxSubsteps < 1 || profile.MaxSubsteps > 4096 {
		return errors.New("max substeps must be between 1 and 4096")
	}
	if err := profile.Personality.Validate(); err != nil {
		return err
	}
	if len(profile.Dynamics) != len(allEmotions) ||
		len(profile.PersonalityInfluences) != len(allEmotions) ||
		len(profile.PhysiologyInfluences) != len(allEmotions) {
		return errors.New("profile requires complete emotion matrices")
	}
	for index, emotion := range allEmotions {
		if profile.Dynamics[index].Emotion != emotion ||
			profile.PersonalityInfluences[index].Emotion != emotion ||
			profile.PhysiologyInfluences[index].Emotion != emotion {
			return errors.New("emotion matrices must use canonical order")
		}
		if err := profile.Dynamics[index].Validate(); err != nil {
			return err
		}
		if err := profile.PersonalityInfluences[index].Validate(); err != nil {
			return err
		}
		if err := profile.PhysiologyInfluences[index].Validate(); err != nil {
			return err
		}
	}
	last := ""
	for _, interaction := range profile.Interactions {
		if err := interaction.Validate(); err != nil {
			return err
		}
		key := interaction.Key()
		if key <= last {
			return errors.New("interactions must be sorted and unique")
		}
		last = key
	}
	if err := validateCompleteDrives(profile.Drives); err != nil {
		return err
	}
	if err := validateCompleteComplexStates(profile.ComplexStates); err != nil {
		return err
	}
	definitionIDs := make(map[string]struct{}, len(profile.Drives)+len(profile.ComplexStates))
	for _, definition := range profile.Drives {
		if _, exists := definitionIDs[definition.DefinitionID]; exists {
			return errors.New("definition IDs must be unique")
		}
		definitionIDs[definition.DefinitionID] = struct{}{}
	}
	for _, definition := range profile.ComplexStates {
		if _, exists := definitionIDs[definition.DefinitionID]; exists {
			return errors.New("definition IDs must be unique")
		}
		definitionIDs[definition.DefinitionID] = struct{}{}
	}
	return nil
}

func validateCompleteDrives(definitions []DriveDefinition) error {
	expected := [...]DriveKind{
		DriveCognition,
		DriveCoherence,
		DriveRecovery,
		DriveSafety,
		DriveSocialConnection,
	}
	if len(definitions) != len(expected) {
		return fmt.Errorf("profile requires exactly %d drive definitions", len(expected))
	}
	for index, kind := range expected {
		if definitions[index].Kind != kind {
			return errors.New("drive definitions must use canonical order")
		}
		if err := definitions[index].Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateCompleteComplexStates(definitions []ComplexStateDefinition) error {
	expected := [...]ComplexStateKind{
		ComplexStateChronicStress,
		ComplexStateDepressive,
		ComplexStateEmotionalExhaustion,
		ComplexStateEuphoria,
		ComplexStateGuardedAttachment,
	}
	if len(definitions) != len(expected) {
		return fmt.Errorf("profile requires exactly %d complex state definitions", len(expected))
	}
	for index, kind := range expected {
		if definitions[index].Kind != kind {
			return errors.New("complex state definitions must use canonical order")
		}
		if err := definitions[index].Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validPhysiologyField(value string) bool {
	return oneOf(value, "arousal", "energy", "fatigue", "stability", "stress_load")
}

func validRelationshipField(value string) bool {
	return oneOf(value,
		"attachment",
		"confidence_in_user",
		"openness",
		"perceived_safety",
		"tension",
		"unresolved_hurt",
	)
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
