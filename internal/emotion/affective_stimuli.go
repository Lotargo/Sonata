package emotion

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Lotargo/Sonata/internal/config"
)

var allStimulusKinds = [...]StimulusKind{
	StimulusConversationBreak,
	StimulusConversationReturn,
	StimulusPromiseBroken,
	StimulusPromiseKept,
	StimulusResponseAppreciated,
	StimulusResponseRejected,
	StimulusToolFailure,
	StimulusToolSuccess,
	StimulusUserApology,
	StimulusUserBoundary,
	StimulusUserDistress,
	StimulusUserHostility,
	StimulusUserRejection,
	StimulusUserSuccess,
	StimulusUserTrust,
	StimulusUserWarmth,
}

type NamedSignedEffect struct {
	Name   string
	Weight SignedUnit
}

type StimulusDefinition struct {
	Kind                StimulusKind
	DefinitionID        string
	EmotionEffects      []EmotionWeight
	RelationshipEffects []NamedSignedEffect
	PhysiologyEffects   []NamedSignedEffect
}

func (definition StimulusDefinition) Validate() error {
	if !definition.Kind.Valid() {
		return fmt.Errorf("invalid stimulus kind %q", definition.Kind)
	}
	if strings.TrimSpace(definition.DefinitionID) == "" {
		return fmt.Errorf("stimulus %s definition ID is required", definition.Kind)
	}
	if len(definition.EmotionEffects)+len(definition.RelationshipEffects)+len(definition.PhysiologyEffects) == 0 {
		return fmt.Errorf("stimulus %s requires at least one effect", definition.Kind)
	}
	last := ""
	for _, effect := range definition.EmotionEffects {
		if !effect.Emotion.Valid() {
			return fmt.Errorf("stimulus %s has invalid emotion %q", definition.Kind, effect.Emotion)
		}
		if err := effect.Weight.Validate(); err != nil {
			return err
		}
		key := string(effect.Emotion)
		if key <= last {
			return errors.New("stimulus emotion effects must be sorted and unique")
		}
		last = key
	}
	last = ""
	for _, effect := range definition.RelationshipEffects {
		if !validRelationshipField(effect.Name) {
			return fmt.Errorf("stimulus %s has invalid relationship field %q", definition.Kind, effect.Name)
		}
		if err := effect.Weight.Validate(); err != nil {
			return err
		}
		if effect.Name <= last {
			return errors.New("stimulus relationship effects must be sorted and unique")
		}
		last = effect.Name
	}
	last = ""
	for _, effect := range definition.PhysiologyEffects {
		if !validPhysiologyField(effect.Name) {
			return fmt.Errorf("stimulus %s has invalid physiology field %q", definition.Kind, effect.Name)
		}
		if err := effect.Weight.Validate(); err != nil {
			return err
		}
		if effect.Name <= last {
			return errors.New("stimulus physiology effects must be sorted and unique")
		}
		last = effect.Name
	}
	return nil
}

type AffectiveStimulusProfile struct {
	ProfileVersion string
	Definitions    []StimulusDefinition
}

func NewAffectiveStimulusProfileFromConfig(value config.EmotionConfig) (AffectiveStimulusProfile, error) {
	profile := AffectiveStimulusProfile{ProfileVersion: strings.TrimSpace(value.Affective.ProfileVersion)}
	for _, kind := range allStimulusKinds {
		raw, exists := value.Affective.Stimuli[string(kind)]
		if !exists {
			return AffectiveStimulusProfile{}, fmt.Errorf("missing affective stimulus definition for %s", kind)
		}
		definition, err := stimulusDefinitionFromConfig(kind, raw)
		if err != nil {
			return AffectiveStimulusProfile{}, err
		}
		profile.Definitions = append(profile.Definitions, definition)
	}
	if err := profile.Validate(); err != nil {
		return AffectiveStimulusProfile{}, err
	}
	return profile, nil
}

func (profile AffectiveStimulusProfile) Validate() error {
	if strings.TrimSpace(profile.ProfileVersion) == "" {
		return errors.New("stimulus profile version is required")
	}
	if len(profile.Definitions) != len(allStimulusKinds) {
		return fmt.Errorf("stimulus profile requires exactly %d definitions", len(allStimulusKinds))
	}
	seenIDs := make(map[string]struct{}, len(profile.Definitions))
	for index, kind := range allStimulusKinds {
		definition := profile.Definitions[index]
		if definition.Kind != kind {
			return errors.New("stimulus definitions must use canonical order")
		}
		if err := definition.Validate(); err != nil {
			return err
		}
		if _, exists := seenIDs[definition.DefinitionID]; exists {
			return errors.New("stimulus definition IDs must be unique")
		}
		seenIDs[definition.DefinitionID] = struct{}{}
	}
	return nil
}

func (profile AffectiveStimulusProfile) Definition(kind StimulusKind) (StimulusDefinition, bool) {
	for index, candidate := range allStimulusKinds {
		if candidate == kind {
			return profile.Definitions[index], true
		}
	}
	return StimulusDefinition{}, false
}

type AffectiveRuntimeProfile struct {
	Dynamics AffectiveProfile
	Initial  AffectiveInitialProfile
	Stimuli  AffectiveStimulusProfile
}

func NewAffectiveRuntimeProfileFromConfig(value config.EmotionConfig) (AffectiveRuntimeProfile, error) {
	dynamics, err := NewAffectiveProfileFromConfig(value)
	if err != nil {
		return AffectiveRuntimeProfile{}, err
	}
	initial, err := NewAffectiveInitialProfileFromConfig(value)
	if err != nil {
		return AffectiveRuntimeProfile{}, err
	}
	stimuli, err := NewAffectiveStimulusProfileFromConfig(value)
	if err != nil {
		return AffectiveRuntimeProfile{}, err
	}
	profile := AffectiveRuntimeProfile{Dynamics: dynamics, Initial: initial, Stimuli: stimuli}
	if err := profile.Validate(); err != nil {
		return AffectiveRuntimeProfile{}, err
	}
	return profile, nil
}

func (profile AffectiveRuntimeProfile) Validate() error {
	if err := profile.Dynamics.Validate(); err != nil {
		return err
	}
	if err := profile.Initial.Validate(); err != nil {
		return err
	}
	if err := profile.Stimuli.Validate(); err != nil {
		return err
	}
	version := profile.Dynamics.Version
	if profile.Initial.ProfileVersion != version || profile.Stimuli.ProfileVersion != version {
		return errors.New("affective runtime profile versions do not match")
	}
	seen := make(map[string]string)
	for _, definition := range profile.Dynamics.Drives {
		if owner, exists := seen[definition.DefinitionID]; exists {
			return fmt.Errorf("definition ID %q is shared by %s and drive %s", definition.DefinitionID, owner, definition.Kind)
		}
		seen[definition.DefinitionID] = "drive " + string(definition.Kind)
	}
	for _, definition := range profile.Dynamics.ComplexStates {
		if owner, exists := seen[definition.DefinitionID]; exists {
			return fmt.Errorf("definition ID %q is shared by %s and complex state %s", definition.DefinitionID, owner, definition.Kind)
		}
		seen[definition.DefinitionID] = "complex state " + string(definition.Kind)
	}
	for _, definition := range profile.Stimuli.Definitions {
		if owner, exists := seen[definition.DefinitionID]; exists {
			return fmt.Errorf("definition ID %q is shared by %s and stimulus %s", definition.DefinitionID, owner, definition.Kind)
		}
		seen[definition.DefinitionID] = "stimulus " + string(definition.Kind)
	}
	return nil
}

func stimulusDefinitionFromConfig(kind StimulusKind, value config.StimulusDefinitionConfig) (StimulusDefinition, error) {
	definition := StimulusDefinition{Kind: kind, DefinitionID: strings.TrimSpace(value.DefinitionID)}
	for _, key := range sortedKeys(value.EmotionEffects) {
		weight, err := NewSignedUnit(value.EmotionEffects[key])
		if err != nil {
			return StimulusDefinition{}, err
		}
		definition.EmotionEffects = append(definition.EmotionEffects, EmotionWeight{Emotion: Emotion(key), Weight: weight})
	}
	for _, key := range sortedKeys(value.RelationshipEffects) {
		weight, err := NewSignedUnit(value.RelationshipEffects[key])
		if err != nil {
			return StimulusDefinition{}, err
		}
		definition.RelationshipEffects = append(definition.RelationshipEffects, NamedSignedEffect{Name: key, Weight: weight})
	}
	for _, key := range sortedKeys(value.PhysiologyEffects) {
		weight, err := NewSignedUnit(value.PhysiologyEffects[key])
		if err != nil {
			return StimulusDefinition{}, err
		}
		definition.PhysiologyEffects = append(definition.PhysiologyEffects, NamedSignedEffect{Name: key, Weight: weight})
	}
	return definition, nil
}

func stableStimulusDefinitions(values []StimulusDefinition) []StimulusDefinition {
	result := append([]StimulusDefinition(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		return result[left].Kind < result[right].Kind
	})
	return result
}
