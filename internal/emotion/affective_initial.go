package emotion

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/config"
)

type DriveInitialState struct {
	Kind         DriveKind
	Level        Unit
	Satisfaction Unit
	Urgency      Unit
}

func (state DriveInitialState) Validate() error {
	if !state.Kind.Valid() {
		return fmt.Errorf("invalid initial drive kind %q", state.Kind)
	}
	if err := state.Level.Validate(); err != nil {
		return fmt.Errorf("initial drive %s level: %w", state.Kind, err)
	}
	if err := state.Satisfaction.Validate(); err != nil {
		return fmt.Errorf("initial drive %s satisfaction: %w", state.Kind, err)
	}
	if err := state.Urgency.Validate(); err != nil {
		return fmt.Errorf("initial drive %s urgency: %w", state.Kind, err)
	}
	return nil
}

type NamedNonNegative struct {
	Name  string
	Value NonNegative
}

func (value NamedNonNegative) Validate() error {
	if !validPhysiologyField(value.Name) {
		return fmt.Errorf("invalid physiology recovery field %q", value.Name)
	}
	if err := value.Value.Validate(); err != nil {
		return fmt.Errorf("physiology recovery %s: %w", value.Name, err)
	}
	return nil
}

type AffectiveInitialProfile struct {
	ProfileVersion     string
	Physiology         Physiology
	PhysiologyRecovery []NamedNonNegative
	Drives             []DriveInitialState
}

func NewAffectiveInitialProfileFromConfig(value config.EmotionConfig) (AffectiveInitialProfile, error) {
	physiology, err := physiologyFromInitialConfig(value.Affective.InitialPhysiology)
	if err != nil {
		return AffectiveInitialProfile{}, err
	}
	profile := AffectiveInitialProfile{
		ProfileVersion: strings.TrimSpace(value.Affective.ProfileVersion),
		Physiology:     physiology,
	}
	for _, key := range sortedKeys(value.Affective.PhysiologyRecoveryRates) {
		rate, err := NewNonNegative(value.Affective.PhysiologyRecoveryRates[key])
		if err != nil {
			return AffectiveInitialProfile{}, fmt.Errorf("physiology recovery %s: %w", key, err)
		}
		profile.PhysiologyRecovery = append(profile.PhysiologyRecovery, NamedNonNegative{Name: key, Value: rate})
	}
	keys := make([]string, 0, len(value.Affective.Drives))
	for key := range value.Affective.Drives {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		definition := value.Affective.Drives[key]
		level, err := NewUnit(definition.Baseline)
		if err != nil {
			return AffectiveInitialProfile{}, fmt.Errorf("initial drive %s level: %w", key, err)
		}
		satisfaction, err := NewUnit(definition.InitialSatisfaction)
		if err != nil {
			return AffectiveInitialProfile{}, fmt.Errorf("initial drive %s satisfaction: %w", key, err)
		}
		urgency, err := NewClampedUnit(level.Float64() * (1 - satisfaction.Float64()))
		if err != nil {
			return AffectiveInitialProfile{}, fmt.Errorf("initial drive %s urgency: %w", key, err)
		}
		profile.Drives = append(profile.Drives, DriveInitialState{
			Kind:         DriveKind(key),
			Level:        level,
			Satisfaction: satisfaction,
			Urgency:      urgency,
		})
	}
	if err := profile.Validate(); err != nil {
		return AffectiveInitialProfile{}, err
	}
	return profile, nil
}

func (profile AffectiveInitialProfile) Validate() error {
	if strings.TrimSpace(profile.ProfileVersion) == "" {
		return errors.New("initial affective profile version is required")
	}
	if err := profile.Physiology.Validate(); err != nil {
		return fmt.Errorf("validate initial physiology: %w", err)
	}
	expectedPhysiology := [...]string{"arousal", "energy", "fatigue", "stability", "stress_load"}
	if len(profile.PhysiologyRecovery) != len(expectedPhysiology) {
		return fmt.Errorf("initial profile requires exactly %d physiology recovery rates", len(expectedPhysiology))
	}
	for index, name := range expectedPhysiology {
		if profile.PhysiologyRecovery[index].Name != name {
			return errors.New("physiology recovery rates must use canonical order")
		}
		if err := profile.PhysiologyRecovery[index].Validate(); err != nil {
			return err
		}
	}
	if len(profile.Drives) != len(allDriveKinds) {
		return fmt.Errorf("initial profile requires exactly %d drives", len(allDriveKinds))
	}
	for index, kind := range allDriveKinds {
		if profile.Drives[index].Kind != kind {
			return errors.New("initial drives must use canonical order")
		}
		if err := profile.Drives[index].Validate(); err != nil {
			return err
		}
	}
	return nil
}

func NewBaselineAffectiveStateFromProfiles(
	key StateKey,
	profile AffectiveProfile,
	initial AffectiveInitialProfile,
	relationship RelationshipState,
	at time.Time,
) (AffectiveState, error) {
	if err := profile.Validate(); err != nil {
		return AffectiveState{}, fmt.Errorf("validate affective profile: %w", err)
	}
	if err := initial.Validate(); err != nil {
		return AffectiveState{}, fmt.Errorf("validate affective initial profile: %w", err)
	}
	if profile.Version != initial.ProfileVersion {
		return AffectiveState{}, errors.New("affective dynamics and initial profile versions do not match")
	}
	var emotions Vector
	for _, dynamics := range profile.Dynamics {
		emotions.Set(dynamics.Emotion, dynamics.Baseline.Float64())
	}
	drives := make([]DriveState, 0, len(initial.Drives))
	for _, drive := range initial.Drives {
		drives = append(drives, DriveState(drive))
	}
	state := AffectiveState{
		Key:            key,
		ProfileVersion: profile.Version,
		Emotions:       emotions,
		Physiology:     initial.Physiology,
		Relationship:   relationship,
		Drives:         drives,
		LastUpdatedAt:  at.UTC(),
		Version:        0,
	}
	if err := state.Validate(); err != nil {
		return AffectiveState{}, err
	}
	return state, nil
}

func physiologyFromInitialConfig(value config.AffectivePhysiologyConfig) (Physiology, error) {
	fatigue, err := NewUnit(value.Fatigue)
	if err != nil {
		return Physiology{}, fmt.Errorf("initial fatigue: %w", err)
	}
	arousal, err := NewUnit(value.Arousal)
	if err != nil {
		return Physiology{}, fmt.Errorf("initial arousal: %w", err)
	}
	energy, err := NewUnit(value.Energy)
	if err != nil {
		return Physiology{}, fmt.Errorf("initial energy: %w", err)
	}
	stressLoad, err := NewUnit(value.StressLoad)
	if err != nil {
		return Physiology{}, fmt.Errorf("initial stress load: %w", err)
	}
	stability, err := NewUnit(value.Stability)
	if err != nil {
		return Physiology{}, fmt.Errorf("initial stability: %w", err)
	}
	return Physiology{
		Fatigue:    fatigue,
		Arousal:    arousal,
		Energy:     energy,
		StressLoad: stressLoad,
		Stability:  stability,
	}, nil
}
