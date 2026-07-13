package emotion

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Lotargo/Sonata/internal/config"
)

func NewAffectiveProfileFromConfig(value config.EmotionConfig) (AffectiveProfile, error) {
	profile := AffectiveProfile{
		Version:         strings.TrimSpace(value.Affective.ProfileVersion),
		IntegrationStep: value.Affective.IntegrationStep.Value(),
		MaxSubsteps:     value.Affective.MaxSubsteps,
	}
	personality, err := personalityFromConfig(value.Affective.Personality)
	if err != nil {
		return AffectiveProfile{}, err
	}
	profile.Personality = personality

	for _, emotion := range allEmotions {
		name := string(emotion)
		dynamicsConfig, exists := value.Affective.Dynamics[name]
		if !exists {
			return AffectiveProfile{}, fmt.Errorf("missing dynamics for %s", emotion)
		}
		baseline, exists := value.Baseline[name]
		if !exists {
			return AffectiveProfile{}, fmt.Errorf("missing baseline for %s", emotion)
		}
		recoveryRate, exists := value.DecayRates[name]
		if !exists {
			return AffectiveProfile{}, fmt.Errorf("missing recovery rate for %s", emotion)
		}
		dynamics, err := emotionDynamicsFromConfig(emotion, baseline, recoveryRate, dynamicsConfig)
		if err != nil {
			return AffectiveProfile{}, err
		}
		profile.Dynamics = append(profile.Dynamics, dynamics)

		personalityConfig, exists := value.Affective.PersonalityInfluences[name]
		if !exists {
			return AffectiveProfile{}, fmt.Errorf("missing personality influence for %s", emotion)
		}
		personalityInfluence, err := personalityInfluenceFromConfig(emotion, personalityConfig)
		if err != nil {
			return AffectiveProfile{}, err
		}
		profile.PersonalityInfluences = append(profile.PersonalityInfluences, personalityInfluence)

		physiologyConfig, exists := value.Affective.PhysiologyInfluences[name]
		if !exists {
			return AffectiveProfile{}, fmt.Errorf("missing physiology influence for %s", emotion)
		}
		physiologyInfluence, err := physiologyInfluenceFromConfig(emotion, physiologyConfig)
		if err != nil {
			return AffectiveProfile{}, err
		}
		profile.PhysiologyInfluences = append(profile.PhysiologyInfluences, physiologyInfluence)
	}

	for _, raw := range value.Affective.Interactions {
		weight, err := NewSignedUnit(raw.Weight)
		if err != nil {
			return AffectiveProfile{}, err
		}
		profile.Interactions = append(profile.Interactions, Interaction{
			From:   Emotion(raw.From),
			To:     Emotion(raw.To),
			Mode:   InteractionMode(raw.Mode),
			Weight: weight,
		})
	}
	sort.Slice(profile.Interactions, func(left, right int) bool {
		return profile.Interactions[left].Key() < profile.Interactions[right].Key()
	})

	for _, key := range sortedKeys(value.Affective.Drives) {
		definition, err := driveDefinitionFromConfig(DriveKind(key), value.Affective.Drives[key])
		if err != nil {
			return AffectiveProfile{}, err
		}
		profile.Drives = append(profile.Drives, definition)
	}
	for _, key := range sortedKeys(value.Affective.ComplexStates) {
		definition, err := complexStateDefinitionFromConfig(ComplexStateKind(key), value.Affective.ComplexStates[key])
		if err != nil {
			return AffectiveProfile{}, err
		}
		profile.ComplexStates = append(profile.ComplexStates, definition)
	}
	if err := profile.Validate(); err != nil {
		return AffectiveProfile{}, err
	}
	return profile, nil
}

func personalityFromConfig(value config.AffectivePersonalityConfig) (Personality, error) {
	openness, err := NewUnit(value.Openness)
	if err != nil {
		return Personality{}, err
	}
	conscientiousness, err := NewUnit(value.Conscientiousness)
	if err != nil {
		return Personality{}, err
	}
	extraversion, err := NewUnit(value.Extraversion)
	if err != nil {
		return Personality{}, err
	}
	agreeableness, err := NewUnit(value.Agreeableness)
	if err != nil {
		return Personality{}, err
	}
	neuroticism, err := NewUnit(value.Neuroticism)
	if err != nil {
		return Personality{}, err
	}
	sensitivity, err := NewUnit(value.Sensitivity)
	if err != nil {
		return Personality{}, err
	}
	inertia, err := NewUnit(value.EmotionalInertia)
	if err != nil {
		return Personality{}, err
	}
	recovery, err := NewUnit(value.RecoveryCapacity)
	if err != nil {
		return Personality{}, err
	}
	return Personality{
		Openness:          openness,
		Conscientiousness: conscientiousness,
		Extraversion:      extraversion,
		Agreeableness:     agreeableness,
		Neuroticism:       neuroticism,
		Sensitivity:       sensitivity,
		EmotionalInertia:  inertia,
		RecoveryCapacity:  recovery,
	}, nil
}

func emotionDynamicsFromConfig(
	emotion Emotion,
	baselineValue float64,
	recoveryValue float64,
	value config.AffectiveEmotionDynamicsConfig,
) (EmotionDynamics, error) {
	baseline, err := NewUnit(baselineValue)
	if err != nil {
		return EmotionDynamics{}, err
	}
	recoveryRate, err := NewNonNegative(recoveryValue)
	if err != nil {
		return EmotionDynamics{}, err
	}
	excitationGain, err := NewNonNegative(value.ExcitationGain)
	if err != nil {
		return EmotionDynamics{}, err
	}
	persistence, err := NewUnit(value.Persistence)
	if err != nil {
		return EmotionDynamics{}, err
	}
	ceiling, err := NewUnit(value.Ceiling)
	if err != nil {
		return EmotionDynamics{}, err
	}
	maxPositiveDelta, err := NewUnit(value.MaxPositiveDelta)
	if err != nil {
		return EmotionDynamics{}, err
	}
	maxNegativeDelta, err := NewUnit(value.MaxNegativeDelta)
	if err != nil {
		return EmotionDynamics{}, err
	}
	return EmotionDynamics{
		Emotion:          emotion,
		Baseline:         baseline,
		ExcitationGain:   excitationGain,
		RecoveryRate:     recoveryRate,
		Persistence:      persistence,
		Ceiling:          ceiling,
		MaxPositiveDelta: maxPositiveDelta,
		MaxNegativeDelta: maxNegativeDelta,
	}, nil
}

func personalityInfluenceFromConfig(
	emotion Emotion,
	value config.PersonalityInfluenceConfig,
) (PersonalityInfluence, error) {
	openness, err := NewSignedUnit(value.Openness)
	if err != nil {
		return PersonalityInfluence{}, err
	}
	conscientiousness, err := NewSignedUnit(value.Conscientiousness)
	if err != nil {
		return PersonalityInfluence{}, err
	}
	extraversion, err := NewSignedUnit(value.Extraversion)
	if err != nil {
		return PersonalityInfluence{}, err
	}
	agreeableness, err := NewSignedUnit(value.Agreeableness)
	if err != nil {
		return PersonalityInfluence{}, err
	}
	neuroticism, err := NewSignedUnit(value.Neuroticism)
	if err != nil {
		return PersonalityInfluence{}, err
	}
	return PersonalityInfluence{
		Emotion:           emotion,
		Openness:          openness,
		Conscientiousness: conscientiousness,
		Extraversion:      extraversion,
		Agreeableness:     agreeableness,
		Neuroticism:       neuroticism,
	}, nil
}

func physiologyInfluenceFromConfig(
	emotion Emotion,
	value config.PhysiologyInfluenceConfig,
) (PhysiologyInfluence, error) {
	fatigue, err := NewSignedUnit(value.Fatigue)
	if err != nil {
		return PhysiologyInfluence{}, err
	}
	arousal, err := NewSignedUnit(value.Arousal)
	if err != nil {
		return PhysiologyInfluence{}, err
	}
	energy, err := NewSignedUnit(value.Energy)
	if err != nil {
		return PhysiologyInfluence{}, err
	}
	stressLoad, err := NewSignedUnit(value.StressLoad)
	if err != nil {
		return PhysiologyInfluence{}, err
	}
	stability, err := NewSignedUnit(value.Stability)
	if err != nil {
		return PhysiologyInfluence{}, err
	}
	return PhysiologyInfluence{
		Emotion:    emotion,
		Fatigue:    fatigue,
		Arousal:    arousal,
		Energy:     energy,
		StressLoad: stressLoad,
		Stability:  stability,
	}, nil
}

func driveDefinitionFromConfig(
	kind DriveKind,
	value config.DriveDefinitionConfig,
) (DriveDefinition, error) {
	baseline, err := NewUnit(value.Baseline)
	if err != nil {
		return DriveDefinition{}, err
	}
	growthRate, err := NewNonNegative(value.GrowthRate)
	if err != nil {
		return DriveDefinition{}, err
	}
	definition := DriveDefinition{
		Kind:         kind,
		DefinitionID: strings.TrimSpace(value.DefinitionID),
		Baseline:     baseline,
		GrowthRate:   growthRate,
	}
	for _, key := range sortedKeys(value.SatisfactionMap) {
		weight, err := NewSignedUnit(value.SatisfactionMap[key])
		if err != nil {
			return DriveDefinition{}, err
		}
		definition.Satisfaction = append(definition.Satisfaction, StimulusDriveEffect{
			Stimulus: StimulusKind(key),
			Weight:   weight,
		})
	}
	for _, key := range sortedKeys(value.EmotionEffects) {
		weight, err := NewSignedUnit(value.EmotionEffects[key])
		if err != nil {
			return DriveDefinition{}, err
		}
		definition.EmotionEffects = append(definition.EmotionEffects, EmotionWeight{
			Emotion: Emotion(key),
			Weight:  weight,
		})
	}
	return definition, nil
}

func complexStateDefinitionFromConfig(
	kind ComplexStateKind,
	value config.ComplexStateDefinitionConfig,
) (ComplexStateDefinition, error) {
	entryConditions, err := conditionsFromConfig(value.EntryConditions)
	if err != nil {
		return ComplexStateDefinition{}, err
	}
	exitConditions, err := conditionsFromConfig(value.ExitConditions)
	if err != nil {
		return ComplexStateDefinition{}, err
	}
	entryThreshold, err := NewUnit(value.EntryThreshold)
	if err != nil {
		return ComplexStateDefinition{}, err
	}
	exitThreshold, err := NewUnit(value.ExitThreshold)
	if err != nil {
		return ComplexStateDefinition{}, err
	}
	effects, err := effectsFromConfig(value.Effects)
	if err != nil {
		return ComplexStateDefinition{}, err
	}
	return ComplexStateDefinition{
		Kind:             kind,
		DefinitionID:     strings.TrimSpace(value.DefinitionID),
		EntryConditions:  entryConditions,
		ExitConditions:   exitConditions,
		MinEntryDuration: value.MinEntryDuration.Value(),
		MinExitDuration:  value.MinExitDuration.Value(),
		EntryThreshold:   entryThreshold,
		ExitThreshold:    exitThreshold,
		Effects:          effects,
	}, nil
}

func conditionsFromConfig(values []config.ConditionConfig) ([]Condition, error) {
	result := make([]Condition, 0, len(values))
	for _, value := range values {
		signal, err := ParseConditionSignal(value.Signal)
		if err != nil {
			return nil, err
		}
		threshold, err := NewUnit(value.Threshold)
		if err != nil {
			return nil, err
		}
		weight, err := NewUnit(value.Weight)
		if err != nil {
			return nil, err
		}
		result = append(result, Condition{
			Signal:    signal,
			Operator:  ConditionOperator(value.Operator),
			Threshold: threshold,
			Weight:    weight,
		})
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Key() < result[right].Key()
	})
	return result, nil
}

func effectsFromConfig(value config.StateEffectsConfig) (StateEffects, error) {
	var effects StateEffects
	var err error
	if effects.EmotionGain, err = emotionMultipliers(value.EmotionGainModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.EmotionRecovery, err = emotionMultipliers(value.EmotionRecoveryModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.EmotionPersistence, err = emotionMultipliers(value.EmotionPersistenceModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.EmotionCeiling, err = emotionMultipliers(value.EmotionCeilingModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.EmotionInhibition, err = emotionMultipliers(value.EmotionInhibitionModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.EmotionTargetShifts, err = emotionShifts(value.EmotionTargetShifts); err != nil {
		return StateEffects{}, err
	}
	if effects.PhysiologyRecovery, err = namedMultipliers(value.PhysiologyRecoveryModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.RelationshipRecovery, err = namedMultipliers(value.RelationshipRecoveryModifiers); err != nil {
		return StateEffects{}, err
	}
	if effects.DriveUrgency, err = driveMultipliers(value.DriveUrgencyModifiers); err != nil {
		return StateEffects{}, err
	}
	effects.ReportBiases = append([]string(nil), value.ReportBiases...)
	sort.Strings(effects.ReportBiases)
	return effects, nil
}

func emotionMultipliers(values map[string]float64) ([]EmotionMultiplier, error) {
	result := make([]EmotionMultiplier, 0, len(values))
	for _, key := range sortedKeys(values) {
		value, err := NewMultiplier(values[key])
		if err != nil {
			return nil, err
		}
		result = append(result, EmotionMultiplier{Emotion: Emotion(key), Value: value})
	}
	return result, nil
}

func emotionShifts(values map[string]float64) ([]EmotionShift, error) {
	result := make([]EmotionShift, 0, len(values))
	for _, key := range sortedKeys(values) {
		value, err := NewSignedUnit(values[key])
		if err != nil {
			return nil, err
		}
		result = append(result, EmotionShift{Emotion: Emotion(key), Value: value})
	}
	return result, nil
}

func namedMultipliers(values map[string]float64) ([]NamedMultiplier, error) {
	result := make([]NamedMultiplier, 0, len(values))
	for _, key := range sortedKeys(values) {
		value, err := NewMultiplier(values[key])
		if err != nil {
			return nil, err
		}
		result = append(result, NamedMultiplier{Name: key, Value: value})
	}
	return result, nil
}

func driveMultipliers(values map[string]float64) ([]DriveMultiplier, error) {
	result := make([]DriveMultiplier, 0, len(values))
	for _, key := range sortedKeys(values) {
		value, err := NewMultiplier(values[key])
		if err != nil {
			return nil, err
		}
		result = append(result, DriveMultiplier{Drive: DriveKind(key), Value: value})
	}
	return result, nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
