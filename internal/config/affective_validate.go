package config

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var affectiveEmotionNames = []string{
	"anger",
	"anticipation",
	"disgust",
	"fear",
	"joy",
	"sadness",
	"surprise",
	"trust",
}

var affectiveDriveNames = []string{
	"cognition",
	"coherence",
	"recovery",
	"safety",
	"social_connection",
}

var affectiveComplexStateNames = []string{
	"chronic_stress",
	"depressive",
	"emotional_exhaustion",
	"euphoria",
	"guarded_attachment",
}

var affectiveStimulusNames = map[string]struct{}{
	"conversation_break": {}, "conversation_return": {}, "promise_broken": {}, "promise_kept": {},
	"response_appreciated": {}, "response_rejected": {}, "tool_failure": {}, "tool_success": {},
	"user_apology": {}, "user_boundary": {}, "user_distress": {}, "user_hostility": {},
	"user_rejection": {}, "user_success": {}, "user_trust": {}, "user_warmth": {},
}

func (config EmotionConfig) ValidateAffective() []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	validateExactFloatMap(&problems, "emotion.baseline", config.Baseline, affectiveEmotionNames, validateUnit)
	validateExactFloatMap(&problems, "emotion.decay_rates", config.DecayRates, affectiveEmotionNames, validateNonNegative)

	affective := config.Affective
	if strings.TrimSpace(affective.ProfileVersion) == "" {
		add("emotion.affective.profile_version is required")
	}
	if affective.IntegrationStep.Value() <= 0 {
		add("emotion.affective.integration_step must be positive")
	}
	if affective.MaxSubsteps < 1 || affective.MaxSubsteps > 4096 {
		add("emotion.affective.max_substeps must be between 1 and 4096")
	}
	validatePersonality(&problems, affective.Personality)
	validateDynamics(&problems, affective.Dynamics, config.Baseline)
	validatePersonalityInfluences(&problems, affective.PersonalityInfluences)
	validatePhysiologyInfluences(&problems, affective.PhysiologyInfluences)
	validateInteractions(&problems, affective.Interactions)
	definitionIDs := make(map[string]string, len(affective.Drives)+len(affective.ComplexStates))
	validateDrives(&problems, affective.Drives, definitionIDs)
	validateComplexStates(&problems, affective.ComplexStates, definitionIDs)

	sort.Strings(problems)
	return problems
}

func validatePersonality(problems *[]string, value AffectivePersonalityConfig) {
	fields := map[string]float64{
		"agreeableness":     value.Agreeableness,
		"conscientiousness": value.Conscientiousness,
		"emotional_inertia": value.EmotionalInertia,
		"extraversion":      value.Extraversion,
		"neuroticism":       value.Neuroticism,
		"openness":          value.Openness,
		"recovery_capacity": value.RecoveryCapacity,
		"sensitivity":       value.Sensitivity,
	}
	for name, field := range fields {
		if !validateUnit(field) {
			*problems = append(*problems, fmt.Sprintf("emotion.affective.personality.%s must be finite and between 0 and 1", name))
		}
	}
}

func validateDynamics(problems *[]string, values map[string]AffectiveEmotionDynamicsConfig, baselines map[string]float64) {
	validateExactKeys(problems, "emotion.affective.dynamics", values, affectiveEmotionNames)
	for name, value := range values {
		prefix := "emotion.affective.dynamics." + name
		if !validateNonNegative(value.ExcitationGain) {
			*problems = append(*problems, prefix+".excitation_gain must be finite and non-negative")
		}
		if !validateUnit(value.Persistence) {
			*problems = append(*problems, prefix+".persistence must be finite and between 0 and 1")
		}
		if !validateUnit(value.Ceiling) {
			*problems = append(*problems, prefix+".ceiling must be finite and between 0 and 1")
		} else if baseline, exists := baselines[name]; exists && validateUnit(baseline) && value.Ceiling < baseline {
			*problems = append(*problems, prefix+".ceiling cannot be below emotion.baseline."+name)
		}
		if !validatePositiveUnit(value.MaxPositiveDelta) {
			*problems = append(*problems, prefix+".max_positive_delta must be finite, greater than 0 and at most 1")
		}
		if !validatePositiveUnit(value.MaxNegativeDelta) {
			*problems = append(*problems, prefix+".max_negative_delta must be finite, greater than 0 and at most 1")
		}
	}
}

func validatePersonalityInfluences(problems *[]string, values map[string]PersonalityInfluenceConfig) {
	validateExactKeys(problems, "emotion.affective.personality_influences", values, affectiveEmotionNames)
	for name, value := range values {
		fields := map[string]float64{
			"agreeableness":     value.Agreeableness,
			"conscientiousness": value.Conscientiousness,
			"extraversion":      value.Extraversion,
			"neuroticism":       value.Neuroticism,
			"openness":          value.Openness,
		}
		for field, number := range fields {
			if !validateSignedUnit(number) {
				*problems = append(*problems, fmt.Sprintf("emotion.affective.personality_influences.%s.%s must be finite and between -1 and 1", name, field))
			}
		}
	}
}

func validatePhysiologyInfluences(problems *[]string, values map[string]PhysiologyInfluenceConfig) {
	validateExactKeys(problems, "emotion.affective.physiology_influences", values, affectiveEmotionNames)
	for name, value := range values {
		fields := map[string]float64{
			"arousal":     value.Arousal,
			"energy":      value.Energy,
			"fatigue":     value.Fatigue,
			"stability":   value.Stability,
			"stress_load": value.StressLoad,
		}
		for field, number := range fields {
			if !validateSignedUnit(number) {
				*problems = append(*problems, fmt.Sprintf("emotion.affective.physiology_influences.%s.%s must be finite and between -1 and 1", name, field))
			}
		}
	}
}

func validateInteractions(problems *[]string, values []EmotionInteractionConfig) {
	last := ""
	for index, value := range values {
		prefix := fmt.Sprintf("emotion.affective.interactions[%d]", index)
		if !containsString(affectiveEmotionNames, value.From) {
			*problems = append(*problems, prefix+".from is not a supported emotion")
		}
		if !containsString(affectiveEmotionNames, value.To) {
			*problems = append(*problems, prefix+".to is not a supported emotion")
		}
		if value.From == value.To {
			*problems = append(*problems, prefix+" cannot target the same emotion")
		}
		if value.Mode != "excite" && value.Mode != "inhibit" {
			*problems = append(*problems, prefix+".mode must be excite or inhibit")
		}
		if !validateSignedUnit(value.Weight) {
			*problems = append(*problems, prefix+".weight must be finite and between -1 and 1")
		}
		if value.Mode == "excite" && value.Weight <= 0 {
			*problems = append(*problems, prefix+" excitation weight must be positive")
		}
		if value.Mode == "inhibit" && value.Weight >= 0 {
			*problems = append(*problems, prefix+" inhibition weight must be negative")
		}
		key := value.From + "|" + value.To + "|" + value.Mode
		if key <= last {
			*problems = append(*problems, "emotion.affective.interactions must use stable ascending order without duplicates")
		}
		last = key
	}
}

func validateDrives(problems *[]string, values map[string]DriveDefinitionConfig, definitionIDs map[string]string) {
	validateExactKeys(problems, "emotion.affective.drives", values, affectiveDriveNames)
	for name, value := range values {
		prefix := "emotion.affective.drives." + name
		validateDefinitionID(problems, prefix, value.DefinitionID, definitionIDs)
		if !validateUnit(value.Baseline) {
			*problems = append(*problems, prefix+".baseline must be finite and between 0 and 1")
		}
		if !validateNonNegative(value.GrowthRate) {
			*problems = append(*problems, prefix+".growth_rate must be finite and non-negative")
		}
		for stimulus, weight := range value.SatisfactionMap {
			if _, exists := affectiveStimulusNames[stimulus]; !exists {
				*problems = append(*problems, prefix+".satisfaction_map contains unsupported stimulus "+stimulus)
			}
			if !validateSignedUnit(weight) {
				*problems = append(*problems, prefix+".satisfaction_map."+stimulus+" must be finite and between -1 and 1")
			}
		}
		for emotion, weight := range value.EmotionEffects {
			if !containsString(affectiveEmotionNames, emotion) {
				*problems = append(*problems, prefix+".emotion_effects contains unsupported emotion "+emotion)
			}
			if !validateSignedUnit(weight) {
				*problems = append(*problems, prefix+".emotion_effects."+emotion+" must be finite and between -1 and 1")
			}
		}
	}
}

func validateComplexStates(problems *[]string, values map[string]ComplexStateDefinitionConfig, definitionIDs map[string]string) {
	validateExactKeys(problems, "emotion.affective.complex_states", values, affectiveComplexStateNames)
	for name, value := range values {
		prefix := "emotion.affective.complex_states." + name
		validateDefinitionID(problems, prefix, value.DefinitionID, definitionIDs)
		validateConditions(problems, prefix+".entry_conditions", value.EntryConditions)
		validateConditions(problems, prefix+".exit_conditions", value.ExitConditions)
		if value.MinEntryDuration.Value() <= 0 {
			*problems = append(*problems, prefix+".min_entry_duration must be positive")
		}
		if value.MinExitDuration.Value() <= 0 {
			*problems = append(*problems, prefix+".min_exit_duration must be positive")
		}
		if !validateUnit(value.EntryThreshold) {
			*problems = append(*problems, prefix+".entry_threshold must be finite and between 0 and 1")
		}
		if !validateUnit(value.ExitThreshold) {
			*problems = append(*problems, prefix+".exit_threshold must be finite and between 0 and 1")
		}
		if value.EntryThreshold <= value.ExitThreshold {
			*problems = append(*problems, prefix+".entry_threshold must exceed exit_threshold for hysteresis")
		}
		validateEffects(problems, prefix+".effects", value.Effects)
	}
}

func validateConditions(problems *[]string, prefix string, values []ConditionConfig) {
	if len(values) == 0 {
		*problems = append(*problems, prefix+" cannot be empty")
		return
	}
	last := ""
	for index, value := range values {
		itemPrefix := fmt.Sprintf("%s[%d]", prefix, index)
		if !validConditionSignal(value.Signal) {
			*problems = append(*problems, itemPrefix+".signal is invalid")
		}
		if value.Operator != ">=" && value.Operator != "<=" {
			*problems = append(*problems, itemPrefix+".operator must be >= or <=")
		}
		if !validateUnit(value.Threshold) {
			*problems = append(*problems, itemPrefix+".threshold must be finite and between 0 and 1")
		}
		if !validatePositiveUnit(value.Weight) {
			*problems = append(*problems, itemPrefix+".weight must be finite, greater than 0 and at most 1")
		}
		key := value.Signal + "|" + value.Operator
		if key <= last {
			*problems = append(*problems, prefix+" must use stable ascending order without duplicates")
		}
		last = key
	}
}

func validateEffects(problems *[]string, prefix string, effects StateEffectsConfig) {
	for name, values := range map[string]map[string]float64{
		"emotion_ceiling_modifiers":     effects.EmotionCeilingModifiers,
		"emotion_gain_modifiers":        effects.EmotionGainModifiers,
		"emotion_inhibition_modifiers":  effects.EmotionInhibitionModifiers,
		"emotion_persistence_modifiers": effects.EmotionPersistenceModifiers,
		"emotion_recovery_modifiers":    effects.EmotionRecoveryModifiers,
	} {
		validateSubsetFloatMap(problems, prefix+"."+name, values, affectiveEmotionNames, validateMultiplier)
	}
	validateSubsetFloatMap(problems, prefix+".emotion_target_shifts", effects.EmotionTargetShifts, affectiveEmotionNames, validateSignedUnit)
	validateSubsetFloatMap(problems, prefix+".physiology_recovery_modifiers", effects.PhysiologyRecoveryModifiers,
		[]string{"arousal", "energy", "fatigue", "stability", "stress_load"}, validateMultiplier)
	validateSubsetFloatMap(problems, prefix+".relationship_recovery_modifiers", effects.RelationshipRecoveryModifiers,
		[]string{"attachment", "confidence_in_user", "openness", "perceived_safety", "tension", "unresolved_hurt"}, validateMultiplier)
	validateSubsetFloatMap(problems, prefix+".drive_urgency_modifiers", effects.DriveUrgencyModifiers, affectiveDriveNames, validateMultiplier)
	if !sort.StringsAreSorted(effects.ReportBiases) {
		*problems = append(*problems, prefix+".report_biases must use stable ascending order")
	}
	last := ""
	for _, bias := range effects.ReportBiases {
		if strings.TrimSpace(bias) == "" {
			*problems = append(*problems, prefix+".report_biases cannot contain empty values")
		}
		if bias == last {
			*problems = append(*problems, prefix+".report_biases cannot contain duplicates")
		}
		last = bias
	}
}

func validateDefinitionID(problems *[]string, prefix, value string, seen map[string]string) {
	value = strings.TrimSpace(value)
	if value == "" {
		*problems = append(*problems, prefix+".definition_id is required")
		return
	}
	if owner, exists := seen[value]; exists {
		*problems = append(*problems, fmt.Sprintf("%s.definition_id duplicates %s", prefix, owner))
		return
	}
	seen[value] = prefix
}

func validateExactFloatMap(problems *[]string, prefix string, values map[string]float64, expected []string, validate func(float64) bool) {
	validateExactKeys(problems, prefix, values, expected)
	for key, value := range values {
		if !validate(value) {
			*problems = append(*problems, prefix+"."+key+" has an invalid numeric value")
		}
	}
}

func validateSubsetFloatMap(problems *[]string, prefix string, values map[string]float64, allowed []string, validate func(float64) bool) {
	for key, value := range values {
		if !containsString(allowed, key) {
			*problems = append(*problems, prefix+" contains unsupported key "+key)
		}
		if !validate(value) {
			*problems = append(*problems, prefix+"."+key+" has an invalid numeric value")
		}
	}
}

func validateExactKeys[V any](problems *[]string, prefix string, values map[string]V, expected []string) {
	for _, key := range expected {
		if _, exists := values[key]; !exists {
			*problems = append(*problems, prefix+" is missing "+key)
		}
	}
	for key := range values {
		if !containsString(expected, key) {
			*problems = append(*problems, prefix+" contains unsupported key "+key)
		}
	}
}

func validConditionSignal(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) == 2 {
		switch parts[0] {
		case "emotion":
			return containsString(affectiveEmotionNames, parts[1])
		case "physiology":
			return containsString([]string{"arousal", "energy", "fatigue", "stability", "stress_load"}, parts[1])
		case "relationship":
			return containsString([]string{"attachment", "confidence_in_user", "openness", "perceived_safety", "tension", "unresolved_hurt"}, parts[1])
		}
	}
	if len(parts) == 3 && parts[0] == "drive" {
		return containsString(affectiveDriveNames, parts[1]) && containsString([]string{"level", "satisfaction", "urgency"}, parts[2])
	}
	return false
}

func validateUnit(value float64) bool {
	return finiteNumber(value) && value >= 0 && value <= 1
}

func validatePositiveUnit(value float64) bool {
	return validateUnit(value) && value > 0
}

func validateSignedUnit(value float64) bool {
	return finiteNumber(value) && value >= -1 && value <= 1
}

func validateNonNegative(value float64) bool {
	return finiteNumber(value) && value >= 0
}

func validateMultiplier(value float64) bool {
	return finiteNumber(value) && value >= 0 && value <= 2
}

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
