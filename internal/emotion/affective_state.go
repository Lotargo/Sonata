package emotion

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// AffectiveState is the v1 canonical state envelope.
// The existing State type remains the v0 compatibility state until the
// transition engine and repository are migrated as one verified change.
type AffectiveState struct {
	Key            StateKey
	ProfileVersion string
	Emotions       Vector
	Physiology     Physiology
	Relationship   RelationshipState
	Drives         []DriveState
	ComplexStates  []ComplexState
	Evidence       []StateEvidence
	LastUpdatedAt  time.Time
	Version        int64
}

func NewBaselineAffectiveState(
	key StateKey,
	profileVersion string,
	emotions Vector,
	relationship RelationshipState,
	at time.Time,
) (AffectiveState, error) {
	state := AffectiveState{
		Key:            key,
		ProfileVersion: strings.TrimSpace(profileVersion),
		Emotions:       emotions,
		Physiology:     BaselinePhysiology(),
		Relationship:   relationship,
		Drives:         BaselineDrives(),
		ComplexStates:  nil,
		Evidence:       nil,
		LastUpdatedAt:  at.UTC(),
		Version:        0,
	}
	if err := state.Validate(); err != nil {
		return AffectiveState{}, err
	}
	return state, nil
}

func (state AffectiveState) Validate() error {
	if err := state.Key.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(state.ProfileVersion) == "" {
		return errors.New("affective profile version is required")
	}
	if err := state.Emotions.Validate(); err != nil {
		return fmt.Errorf("validate affective emotions: %w", err)
	}
	if err := state.Physiology.Validate(); err != nil {
		return fmt.Errorf("validate affective physiology: %w", err)
	}
	if err := state.Relationship.Validate(); err != nil {
		return fmt.Errorf("validate affective relationship: %w", err)
	}
	if err := ValidateDrives(state.Drives); err != nil {
		return fmt.Errorf("validate affective drives: %w", err)
	}
	if err := ValidateComplexStates(state.ComplexStates); err != nil {
		return fmt.Errorf("validate affective complex states: %w", err)
	}
	if err := ValidateStateEvidence(state.Evidence); err != nil {
		return fmt.Errorf("validate affective evidence: %w", err)
	}
	if state.LastUpdatedAt.IsZero() {
		return errors.New("affective last update time is required")
	}
	if state.Version < 0 {
		return errors.New("affective state version cannot be negative")
	}
	return nil
}

// Clone returns a deep copy so repositories cannot leak mutable slice backing
// arrays between concurrent transitions.
func (state AffectiveState) Clone() AffectiveState {
	clone := state
	clone.Drives = append([]DriveState(nil), state.Drives...)
	clone.ComplexStates = append([]ComplexState(nil), state.ComplexStates...)
	clone.Evidence = append([]StateEvidence(nil), state.Evidence...)
	return clone
}
