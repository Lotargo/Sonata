package emotion

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

type TransitionLog struct {
	ProfileVersion      string
	RelationshipRule    string
	FromVersion         int64
	ToVersion           int64
	Elapsed             time.Duration
	RecoverySegments    int
	AppliedStimuli      int
	IntegrationSubsteps int
	StimulusDefinitions []string
}

type indexedStimulus struct {
	index    int
	stimulus Stimulus
}

// Transition is a pure deterministic state transition. It performs no storage,
// LLM, tool or network operations.
func Transition(
	previous AffectiveState,
	stimuli []Stimulus,
	now time.Time,
	profile AffectiveRuntimeProfile,
) (AffectiveState, TransitionLog, error) {
	if err := profile.Validate(); err != nil {
		return AffectiveState{}, TransitionLog{}, fmt.Errorf("validate runtime profile: %w", err)
	}
	relationshipRuleID, err := relationshipResponseRuleID(profile.Dynamics.Version)
	if err != nil {
		return AffectiveState{}, TransitionLog{}, fmt.Errorf("validate relationship response rule: %w", err)
	}
	if err := previous.Validate(); err != nil {
		return AffectiveState{}, TransitionLog{}, fmt.Errorf("validate previous affective state: %w", err)
	}
	now = now.UTC()
	if now.Before(previous.LastUpdatedAt) {
		return AffectiveState{}, TransitionLog{}, errors.New("transition time cannot precede state time")
	}
	if previous.ProfileVersion != profile.Dynamics.Version {
		return AffectiveState{}, TransitionLog{}, errors.New("state profile version does not match runtime profile")
	}
	if err := validateAffectiveStateDefinitions(previous, profile.Dynamics); err != nil {
		return AffectiveState{}, TransitionLog{}, err
	}

	ordered := make([]indexedStimulus, 0, len(stimuli))
	for index, stimulus := range stimuli {
		if err := stimulus.Validate(); err != nil {
			return AffectiveState{}, TransitionLog{}, fmt.Errorf("validate stimulus %d: %w", index, err)
		}
		if stimulus.CreatedAt.IsZero() {
			return AffectiveState{}, TransitionLog{}, fmt.Errorf("stimulus %d timestamp is required", index)
		}
		stimulus.CreatedAt = stimulus.CreatedAt.UTC()
		if stimulus.CreatedAt.Before(previous.LastUpdatedAt) {
			return AffectiveState{}, TransitionLog{}, fmt.Errorf("stimulus %d precedes state time", index)
		}
		if stimulus.CreatedAt.After(now) {
			return AffectiveState{}, TransitionLog{}, fmt.Errorf("stimulus %d is in the future", index)
		}
		ordered = append(ordered, indexedStimulus{index: index, stimulus: stimulus})
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].stimulus.CreatedAt.Equal(ordered[right].stimulus.CreatedAt) {
			return ordered[left].index < ordered[right].index
		}
		return ordered[left].stimulus.CreatedAt.Before(ordered[right].stimulus.CreatedAt)
	})

	next := previous.Clone()
	log := TransitionLog{
		ProfileVersion:   profile.Dynamics.Version,
		RelationshipRule: relationshipRuleID,
		FromVersion:      previous.Version,
		Elapsed:          now.Sub(previous.LastUpdatedAt),
	}
	cursor := previous.LastUpdatedAt
	for _, event := range ordered {
		if event.stimulus.CreatedAt.After(cursor) {
			substeps, err := advanceAffectiveRuntimeState(&next, cursor, event.stimulus.CreatedAt.Sub(cursor), profile)
			if err != nil {
				return AffectiveState{}, TransitionLog{}, err
			}
			log.IntegrationSubsteps += substeps
			log.RecoverySegments++
			cursor = event.stimulus.CreatedAt
		}
		definition, exists := profile.Stimuli.Definition(event.stimulus.Kind)
		if !exists {
			return AffectiveState{}, TransitionLog{}, fmt.Errorf("missing stimulus definition for %s", event.stimulus.Kind)
		}
		if err := applyAffectiveStimulus(&next, event.stimulus, definition, profile.Dynamics); err != nil {
			return AffectiveState{}, TransitionLog{}, err
		}
		log.AppliedStimuli++
		log.StimulusDefinitions = append(log.StimulusDefinitions, definition.DefinitionID)
	}
	if now.After(cursor) {
		substeps, err := advanceAffectiveRuntimeState(&next, cursor, now.Sub(cursor), profile)
		if err != nil {
			return AffectiveState{}, TransitionLog{}, err
		}
		log.IntegrationSubsteps += substeps
		log.RecoverySegments++
	}

	if log.Elapsed > 0 || log.AppliedStimuli > 0 {
		next.LastUpdatedAt = now
		next.Version = previous.Version + 1
	}
	if err := next.Validate(); err != nil {
		return AffectiveState{}, TransitionLog{}, fmt.Errorf("validate next affective state: %w", err)
	}
	log.ToVersion = next.Version
	return next, log, nil
}

func validateAffectiveStateDefinitions(state AffectiveState, profile AffectiveProfile) error {
	for _, active := range state.ComplexStates {
		definition, exists := complexDefinition(profile, active.Kind)
		if !exists {
			return fmt.Errorf("state contains unsupported complex state %s", active.Kind)
		}
		if active.DefinitionID != definition.DefinitionID {
			return fmt.Errorf("complex state %s definition %q does not match profile definition %q", active.Kind, active.DefinitionID, definition.DefinitionID)
		}
	}
	for _, evidence := range state.Evidence {
		definition, exists := complexDefinition(profile, evidence.Kind)
		if !exists {
			return fmt.Errorf("state contains unsupported evidence %s", evidence.Kind)
		}
		if evidence.DefinitionID != definition.DefinitionID {
			return fmt.Errorf("evidence %s definition %q does not match profile definition %q", evidence.Kind, evidence.DefinitionID, definition.DefinitionID)
		}
	}
	return nil
}

func applyAffectiveStimulus(
	state *AffectiveState,
	stimulus Stimulus,
	definition StimulusDefinition,
	profile AffectiveProfile,
) error {
	scale := stimulus.Intensity * stimulus.Confidence
	if scale == 0 {
		updateDriveState(state, stimulus.Kind, scale, profile)
		return nil
	}

	var direct [len(allEmotions)]float64
	for _, effect := range definition.EmotionEffects {
		index := emotionIndex(effect.Emotion)
		dynamics := profile.Dynamics[index]
		modifier := personalityResponse(profile.Personality, profile.PersonalityInfluences[index])
		modifier *= physiologyResponse(state.Physiology, profile.PhysiologyInfluences[index])
		relationshipModifier, err := relationshipResponse(profile.Version, state.Relationship, effect.Emotion, effect.Weight.Float64())
		if err != nil {
			return fmt.Errorf("calculate relationship response for %s: %w", effect.Emotion, err)
		}
		modifier *= relationshipModifier
		modifier *= activeEmotionMultiplier(*state, profile, effect.Emotion, effectGain)
		delta := effect.Weight.Float64() * scale * dynamics.ExcitationGain.Float64() * modifier
		if delta >= 0 {
			delta = math.Min(delta, dynamics.MaxPositiveDelta.Float64())
		} else {
			delta = math.Max(delta, -dynamics.MaxNegativeDelta.Float64())
		}
		direct[index] += delta
	}
	applyEmotionDelta(state, direct, profile)

	var interactions [len(allEmotions)]float64
	for _, interaction := range profile.Interactions {
		sourceIndex := emotionIndex(interaction.From)
		targetIndex := emotionIndex(interaction.To)
		sourceDynamics := profile.Dynamics[sourceIndex]
		sourceExcess := math.Max(0, state.Emotions.Get(interaction.From)-sourceDynamics.Baseline.Float64())
		if sourceExcess == 0 {
			continue
		}
		delta := sourceExcess * interaction.Weight.Float64() * scale
		if delta < 0 {
			delta *= activeEmotionMultiplier(*state, profile, interaction.To, effectInhibition)
		}
		targetDynamics := profile.Dynamics[targetIndex]
		if delta >= 0 {
			delta = math.Min(delta, targetDynamics.MaxPositiveDelta.Float64())
		} else {
			delta = math.Max(delta, -targetDynamics.MaxNegativeDelta.Float64())
		}
		interactions[targetIndex] += delta
	}
	applyEmotionDelta(state, interactions, profile)

	for _, effect := range definition.RelationshipEffects {
		applyRelationshipEffect(&state.Relationship, effect.Name, effect.Weight.Float64()*scale)
	}
	for _, effect := range definition.PhysiologyEffects {
		applyPhysiologyEffect(&state.Physiology, effect.Name, effect.Weight.Float64()*scale)
	}
	updateDriveState(state, stimulus.Kind, scale, profile)
	return nil
}

func advanceAffectiveEmotions(state *AffectiveState, elapsed time.Duration, profile AffectiveProfile) {
	if elapsed <= 0 {
		return
	}
	hours := elapsed.Hours()
	for index, emotion := range allEmotions {
		dynamics := profile.Dynamics[index]
		target := dynamics.Baseline.Float64() + activeEmotionShift(*state, profile, emotion)
		ceiling := effectiveEmotionCeiling(*state, profile, emotion)
		target = clampFloat(target, 0, ceiling)
		rate := dynamics.RecoveryRate.Float64()
		rate *= 0.5 + profile.Personality.RecoveryCapacity.Float64()
		rate *= 1 - 0.75*dynamics.Persistence.Float64()
		rate *= 1 - 0.5*profile.Personality.EmotionalInertia.Float64()
		rate *= activeEmotionMultiplier(*state, profile, emotion, effectRecovery)
		if rate < 0 {
			rate = 0
		}
		current := state.Emotions.Get(emotion)
		value := target + (current-target)*math.Exp(-rate*hours)
		state.Emotions.Set(emotion, clampFloat(value, 0, ceiling))
	}
}

func applyEmotionDelta(state *AffectiveState, deltas [len(allEmotions)]float64, profile AffectiveProfile) {
	for index, emotion := range allEmotions {
		ceiling := effectiveEmotionCeiling(*state, profile, emotion)
		value := state.Emotions.Get(emotion) + deltas[index]
		state.Emotions.Set(emotion, clampFloat(value, 0, ceiling))
	}
}

func personalityResponse(personality Personality, influence PersonalityInfluence) float64 {
	modifier := 1.0
	modifier += (personality.Openness.Float64() - 0.5) * influence.Openness.Float64()
	modifier += (personality.Conscientiousness.Float64() - 0.5) * influence.Conscientiousness.Float64()
	modifier += (personality.Extraversion.Float64() - 0.5) * influence.Extraversion.Float64()
	modifier += (personality.Agreeableness.Float64() - 0.5) * influence.Agreeableness.Float64()
	modifier += (personality.Neuroticism.Float64() - 0.5) * influence.Neuroticism.Float64()
	modifier *= 0.5 + personality.Sensitivity.Float64()
	return clampFloat(modifier, 0.1, 2)
}

func physiologyResponse(physiology Physiology, influence PhysiologyInfluence) float64 {
	modifier := 1.0
	modifier += physiology.Fatigue.Float64() * influence.Fatigue.Float64()
	modifier += (physiology.Arousal.Float64() - 0.5) * influence.Arousal.Float64()
	modifier += (physiology.Energy.Float64() - 0.5) * influence.Energy.Float64()
	modifier += physiology.StressLoad.Float64() * influence.StressLoad.Float64()
	modifier += (physiology.Stability.Float64() - 0.5) * influence.Stability.Float64()
	return clampFloat(modifier, 0.1, 2)
}

type effectSelector int

const (
	effectGain effectSelector = iota
	effectRecovery
	effectInhibition
)

func activeEmotionMultiplier(
	state AffectiveState,
	profile AffectiveProfile,
	emotion Emotion,
	selector effectSelector,
) float64 {
	result := 1.0
	for _, active := range state.ComplexStates {
		if !active.Active() {
			continue
		}
		definition, exists := complexDefinition(profile, active.Kind)
		if !exists || definition.DefinitionID != active.DefinitionID {
			continue
		}
		var modifiers []EmotionMultiplier
		switch selector {
		case effectGain:
			modifiers = definition.Effects.EmotionGain
		case effectRecovery:
			modifiers = definition.Effects.EmotionRecovery
		case effectInhibition:
			modifiers = definition.Effects.EmotionInhibition
		}
		if value, exists := emotionMultiplier(modifiers, emotion); exists {
			result *= interpolateMultiplier(value.Float64(), active.Activation.Float64())
		}
	}
	return clampFloat(result, 0, 2)
}

func activeEmotionShift(state AffectiveState, profile AffectiveProfile, emotion Emotion) float64 {
	result := 0.0
	for _, active := range state.ComplexStates {
		if !active.Active() {
			continue
		}
		definition, exists := complexDefinition(profile, active.Kind)
		if !exists || definition.DefinitionID != active.DefinitionID {
			continue
		}
		for _, shift := range definition.Effects.EmotionTargetShifts {
			if shift.Emotion == emotion {
				result += shift.Value.Float64() * active.Activation.Float64()
			}
		}
	}
	return clampFloat(result, -1, 1)
}

func effectiveEmotionCeiling(state AffectiveState, profile AffectiveProfile, emotion Emotion) float64 {
	ceiling := profile.Dynamics[emotionIndex(emotion)].Ceiling.Float64()
	for _, active := range state.ComplexStates {
		if !active.Active() {
			continue
		}
		definition, exists := complexDefinition(profile, active.Kind)
		if !exists || definition.DefinitionID != active.DefinitionID {
			continue
		}
		if value, exists := emotionMultiplier(definition.Effects.EmotionCeiling, emotion); exists {
			ceiling *= interpolateMultiplier(value.Float64(), active.Activation.Float64())
		}
	}
	return clampFloat(ceiling, 0, 1)
}

func updateDriveState(state *AffectiveState, kind StimulusKind, scale float64, profile AffectiveProfile) {
	for index := range state.Drives {
		if index >= len(profile.Drives) || state.Drives[index].Kind != profile.Drives[index].Kind {
			continue
		}
		definition := profile.Drives[index]
		for _, effect := range definition.Satisfaction {
			if effect.Stimulus == kind {
				state.Drives[index].Satisfaction = Unit(clampFloat(
					state.Drives[index].Satisfaction.Float64()+effect.Weight.Float64()*scale,
					0,
					1,
				))
				break
			}
		}
		urgency := state.Drives[index].Level.Float64() * (1 - state.Drives[index].Satisfaction.Float64())
		urgency *= activeDriveUrgencyMultiplier(*state, profile, state.Drives[index].Kind)
		state.Drives[index].Urgency = Unit(clampFloat(urgency, 0, 1))
	}
}

func activeDriveUrgencyMultiplier(state AffectiveState, profile AffectiveProfile, drive DriveKind) float64 {
	result := 1.0
	for _, active := range state.ComplexStates {
		if !active.Active() {
			continue
		}
		definition, exists := complexDefinition(profile, active.Kind)
		if !exists || definition.DefinitionID != active.DefinitionID {
			continue
		}
		for _, modifier := range definition.Effects.DriveUrgency {
			if modifier.Drive == drive {
				result *= interpolateMultiplier(modifier.Value.Float64(), active.Activation.Float64())
			}
		}
	}
	return clampFloat(result, 0, 2)
}

func interpolateMultiplier(configured, activation float64) float64 {
	return 1 + (configured-1)*clampFloat(activation, 0, 1)
}

func applyRelationshipEffect(state *RelationshipState, name string, delta float64) {
	switch name {
	case "attachment":
		state.Attachment = clampFloat(state.Attachment+delta, 0, 1)
	case "confidence_in_user":
		state.ConfidenceInUser = clampFloat(state.ConfidenceInUser+delta, 0, 1)
	case "openness":
		state.Openness = clampFloat(state.Openness+delta, 0, 1)
	case "perceived_safety":
		state.PerceivedSafety = clampFloat(state.PerceivedSafety+delta, 0, 1)
	case "tension":
		state.Tension = clampFloat(state.Tension+delta, 0, 1)
	case "unresolved_hurt":
		state.UnresolvedHurt = clampFloat(state.UnresolvedHurt+delta, 0, 1)
	}
}

func applyPhysiologyEffect(state *Physiology, name string, delta float64) {
	switch name {
	case "arousal":
		state.Arousal = Unit(clampFloat(state.Arousal.Float64()+delta, 0, 1))
	case "energy":
		state.Energy = Unit(clampFloat(state.Energy.Float64()+delta, 0, 1))
	case "fatigue":
		state.Fatigue = Unit(clampFloat(state.Fatigue.Float64()+delta, 0, 1))
	case "stability":
		state.Stability = Unit(clampFloat(state.Stability.Float64()+delta, 0, 1))
	case "stress_load":
		state.StressLoad = Unit(clampFloat(state.StressLoad.Float64()+delta, 0, 1))
	}
}

func emotionIndex(emotion Emotion) int {
	for index, candidate := range allEmotions {
		if candidate == emotion {
			return index
		}
	}
	return 0
}

func complexDefinition(profile AffectiveProfile, kind ComplexStateKind) (ComplexStateDefinition, bool) {
	for _, definition := range profile.ComplexStates {
		if definition.Kind == kind {
			return definition, true
		}
	}
	return ComplexStateDefinition{}, false
}

func emotionMultiplier(values []EmotionMultiplier, emotion Emotion) (Multiplier, bool) {
	for _, value := range values {
		if value.Emotion == emotion {
			return value.Value, true
		}
	}
	return 0, false
}

func clampFloat(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
