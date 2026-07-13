package emotion

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Personality contains stable response traits. It is not runtime mood.
type Personality struct {
	Openness          Unit
	Conscientiousness Unit
	Extraversion      Unit
	Agreeableness     Unit
	Neuroticism       Unit
	Sensitivity       Unit
	EmotionalInertia  Unit
	RecoveryCapacity  Unit
}

func NeutralPersonality() Personality {
	return Personality{
		Openness:          Unit(0.5),
		Conscientiousness: Unit(0.5),
		Extraversion:      Unit(0.5),
		Agreeableness:     Unit(0.5),
		Neuroticism:       Unit(0.5),
		Sensitivity:       Unit(0.5),
		EmotionalInertia:  Unit(0.5),
		RecoveryCapacity:  Unit(0.5),
	}
}

func (personality Personality) Validate() error {
	values := map[string]Unit{
		"openness":          personality.Openness,
		"conscientiousness": personality.Conscientiousness,
		"extraversion":      personality.Extraversion,
		"agreeableness":     personality.Agreeableness,
		"neuroticism":       personality.Neuroticism,
		"sensitivity":       personality.Sensitivity,
		"emotional_inertia": personality.EmotionalInertia,
		"recovery_capacity": personality.RecoveryCapacity,
	}
	for name, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("personality %s: %w", name, err)
		}
	}
	return nil
}

// Physiology contains runtime conditions that modulate emotional channels.
type Physiology struct {
	Fatigue    Unit
	Arousal    Unit
	Energy     Unit
	StressLoad Unit
	Stability  Unit
}

func BaselinePhysiology() Physiology {
	return Physiology{
		Fatigue:    0,
		Arousal:    Unit(0.2),
		Energy:     1,
		StressLoad: 0,
		Stability:  1,
	}
}

func (physiology Physiology) Validate() error {
	values := map[string]Unit{
		"fatigue":     physiology.Fatigue,
		"arousal":     physiology.Arousal,
		"energy":      physiology.Energy,
		"stress_load": physiology.StressLoad,
		"stability":   physiology.Stability,
	}
	for name, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("physiology %s: %w", name, err)
		}
	}
	return nil
}

type DriveKind string

const (
	DriveCognition        DriveKind = "cognition"
	DriveSocialConnection DriveKind = "social_connection"
	DriveSafety           DriveKind = "safety"
	DriveCoherence        DriveKind = "coherence"
	DriveRecovery         DriveKind = "recovery"
)

// allDriveKinds uses canonical lexical order so serialization, validation and replay agree.
var allDriveKinds = [...]DriveKind{
	DriveCoherence,
	DriveCognition,
	DriveRecovery,
	DriveSafety,
	DriveSocialConnection,
}

func AllDriveKinds() []DriveKind {
	return append([]DriveKind(nil), allDriveKinds[:]...)
}

func (kind DriveKind) Valid() bool {
	switch kind {
	case DriveCognition, DriveSocialConnection, DriveSafety, DriveCoherence, DriveRecovery:
		return true
	default:
		return false
	}
}

// DriveState is a runtime motivational signal and never authorizes actions.
type DriveState struct {
	Kind         DriveKind
	Level        Unit
	Satisfaction Unit
	Urgency      Unit
}

func (drive DriveState) Validate() error {
	if !drive.Kind.Valid() {
		return fmt.Errorf("unsupported drive kind %q", drive.Kind)
	}
	if err := drive.Level.Validate(); err != nil {
		return fmt.Errorf("drive %s level: %w", drive.Kind, err)
	}
	if err := drive.Satisfaction.Validate(); err != nil {
		return fmt.Errorf("drive %s satisfaction: %w", drive.Kind, err)
	}
	if err := drive.Urgency.Validate(); err != nil {
		return fmt.Errorf("drive %s urgency: %w", drive.Kind, err)
	}
	return nil
}

func BaselineDrives() []DriveState {
	return []DriveState{
		{Kind: DriveCoherence, Level: Unit(0.7), Satisfaction: Unit(0.6), Urgency: Unit(0.25)},
		{Kind: DriveCognition, Level: Unit(0.7), Satisfaction: Unit(0.5), Urgency: Unit(0.35)},
		{Kind: DriveRecovery, Level: Unit(0.5), Satisfaction: Unit(0.8), Urgency: Unit(0.10)},
		{Kind: DriveSafety, Level: Unit(0.8), Satisfaction: Unit(0.7), Urgency: Unit(0.20)},
		{Kind: DriveSocialConnection, Level: Unit(0.6), Satisfaction: Unit(0.5), Urgency: Unit(0.30)},
	}
}

func ValidateDrives(drives []DriveState) error {
	if len(drives) == 0 {
		return nil
	}
	seen := make(map[DriveKind]struct{}, len(drives))
	for index, drive := range drives {
		if err := drive.Validate(); err != nil {
			return fmt.Errorf("validate drive %d: %w", index, err)
		}
		if _, exists := seen[drive.Kind]; exists {
			return fmt.Errorf("duplicate drive kind %q", drive.Kind)
		}
		seen[drive.Kind] = struct{}{}
		if index > 0 && drives[index-1].Kind >= drive.Kind {
			return errors.New("drives must use stable ascending kind order")
		}
	}
	return nil
}

type ComplexStateKind string

const (
	ComplexStateDepressive          ComplexStateKind = "depressive"
	ComplexStateEuphoria            ComplexStateKind = "euphoria"
	ComplexStateChronicStress       ComplexStateKind = "chronic_stress"
	ComplexStateEmotionalExhaustion ComplexStateKind = "emotional_exhaustion"
	ComplexStateGuardedAttachment   ComplexStateKind = "guarded_attachment"
)

func (kind ComplexStateKind) Valid() bool {
	switch kind {
	case ComplexStateDepressive,
		ComplexStateEuphoria,
		ComplexStateChronicStress,
		ComplexStateEmotionalExhaustion,
		ComplexStateGuardedAttachment:
		return true
	default:
		return false
	}
}

// ComplexState is a long-lived dynamics mode, not a medical diagnosis.
type ComplexState struct {
	Kind        ComplexStateKind
	Activation  Unit
	ActiveSince time.Time
}

func (state ComplexState) Active() bool {
	return state.Activation > 0
}

func (state ComplexState) Validate() error {
	if !state.Kind.Valid() {
		return fmt.Errorf("unsupported complex state kind %q", state.Kind)
	}
	if err := state.Activation.Validate(); err != nil {
		return fmt.Errorf("complex state %s activation: %w", state.Kind, err)
	}
	if state.Active() && state.ActiveSince.IsZero() {
		return fmt.Errorf("active complex state %s requires active timestamp", state.Kind)
	}
	if !state.Active() && !state.ActiveSince.IsZero() {
		return fmt.Errorf("inactive complex state %s cannot have active timestamp", state.Kind)
	}
	return nil
}

func ValidateComplexStates(states []ComplexState) error {
	seen := make(map[ComplexStateKind]struct{}, len(states))
	for index, state := range states {
		if err := state.Validate(); err != nil {
			return fmt.Errorf("validate complex state %d: %w", index, err)
		}
		if _, exists := seen[state.Kind]; exists {
			return fmt.Errorf("duplicate complex state kind %q", state.Kind)
		}
		seen[state.Kind] = struct{}{}
		if index > 0 && states[index-1].Kind >= state.Kind {
			return errors.New("complex states must use stable ascending kind order")
		}
	}
	return nil
}

// EvidenceAccumulator stores aggregate temporal evidence without raw content.
type EvidenceAccumulator struct {
	PositiveArea  NonNegative
	ViolationArea NonNegative
	ObservedFor   time.Duration
	LastUpdatedAt time.Time
}

func (evidence EvidenceAccumulator) Validate() error {
	if err := evidence.PositiveArea.Validate(); err != nil {
		return fmt.Errorf("positive evidence: %w", err)
	}
	if err := evidence.ViolationArea.Validate(); err != nil {
		return fmt.Errorf("violation evidence: %w", err)
	}
	if evidence.ObservedFor < 0 {
		return errors.New("evidence observed duration cannot be negative")
	}
	if evidence.ObservedFor > 0 && evidence.LastUpdatedAt.IsZero() {
		return errors.New("observed evidence requires last update timestamp")
	}
	return nil
}

type StateEvidence struct {
	Kind     ComplexStateKind
	Evidence EvidenceAccumulator
}

func (evidence StateEvidence) Validate() error {
	if !evidence.Kind.Valid() {
		return fmt.Errorf("unsupported evidence kind %q", evidence.Kind)
	}
	if err := evidence.Evidence.Validate(); err != nil {
		return fmt.Errorf("evidence for %s: %w", evidence.Kind, err)
	}
	return nil
}

func ValidateStateEvidence(values []StateEvidence) error {
	if !sort.SliceIsSorted(values, func(left, right int) bool {
		return values[left].Kind < values[right].Kind
	}) {
		return errors.New("state evidence must use stable ascending kind order")
	}
	seen := make(map[ComplexStateKind]struct{}, len(values))
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("validate state evidence %d: %w", index, err)
		}
		if _, exists := seen[value.Kind]; exists {
			return fmt.Errorf("duplicate state evidence kind %q", value.Kind)
		}
		seen[value.Kind] = struct{}{}
	}
	return nil
}
