package emotion

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

func advanceAffectiveState(state *AffectiveState, from time.Time, elapsed time.Duration, profile AffectiveProfile) (int, error) {
	if elapsed <= 0 {
		return 0, nil
	}
	count := boundedSubstepCount(elapsed, profile.IntegrationStep, profile.MaxSubsteps)
	base := elapsed / time.Duration(count)
	remainder := elapsed % time.Duration(count)
	cursor := from.UTC()
	for index := 0; index < count; index++ {
		step := base
		if time.Duration(index) < remainder {
			step++
		}
		advanceAffectiveEmotions(state, step, profile)
		cursor = cursor.Add(step)
		if err := updateComplexStateEvidence(state, step, cursor, profile); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func boundedSubstepCount(elapsed, configuredStep time.Duration, maximum int) int {
	if elapsed <= 0 {
		return 0
	}
	count := elapsed / configuredStep
	if elapsed%configuredStep != 0 {
		count++
	}
	if count < 1 {
		count = 1
	}
	if count > time.Duration(maximum) {
		count = time.Duration(maximum)
	}
	return int(count)
}

func updateComplexStateEvidence(state *AffectiveState, elapsed time.Duration, at time.Time, profile AffectiveProfile) error {
	if elapsed <= 0 {
		return nil
	}
	ensureStateEvidence(state, profile)
	for definitionIndex, definition := range profile.ComplexStates {
		activeIndex := complexStateIndex(state.ComplexStates, definition.Kind)
		active := activeIndex >= 0
		conditions := definition.EntryConditions
		threshold := definition.EntryThreshold.Float64()
		minimum := definition.MinEntryDuration
		if active {
			conditions = definition.ExitConditions
			threshold = definition.ExitThreshold.Float64()
			minimum = definition.MinExitDuration
		}
		score, err := conditionScore(*state, conditions)
		if err != nil {
			return fmt.Errorf("evaluate %s conditions: %w", definition.Kind, err)
		}
		evidence := &state.Evidence[definitionIndex].Evidence
		if err := accumulateEvidence(evidence, score, threshold, elapsed, minimum, at); err != nil {
			return fmt.Errorf("accumulate %s evidence: %w", definition.Kind, err)
		}
		if evidence.ObservedFor < minimum || evidence.ObservedFor <= 0 {
			continue
		}
		average := evidence.PositiveArea.Float64() / evidence.ObservedFor.Hours()
		if average+1e-12 < threshold {
			continue
		}
		if active {
			state.ComplexStates = append(state.ComplexStates[:activeIndex], state.ComplexStates[activeIndex+1:]...)
		} else {
			activation, err := NewUnit(score)
			if err != nil {
				return err
			}
			state.ComplexStates = append(state.ComplexStates, ComplexState{
				Kind:         definition.Kind,
				DefinitionID: definition.DefinitionID,
				Activation:   activation,
				ActiveSince:  at,
			})
			sort.Slice(state.ComplexStates, func(left, right int) bool {
				return state.ComplexStates[left].Kind < state.ComplexStates[right].Kind
			})
		}
		state.Evidence[definitionIndex].Evidence = EvidenceAccumulator{LastUpdatedAt: at}
	}
	return nil
}

func ensureStateEvidence(state *AffectiveState, profile AffectiveProfile) {
	existing := make(map[ComplexStateKind]StateEvidence, len(state.Evidence))
	for _, evidence := range state.Evidence {
		existing[evidence.Kind] = evidence
	}
	values := make([]StateEvidence, 0, len(profile.ComplexStates))
	for _, definition := range profile.ComplexStates {
		if evidence, exists := existing[definition.Kind]; exists {
			values = append(values, evidence)
			continue
		}
		values = append(values, StateEvidence{
			Kind:         definition.Kind,
			DefinitionID: definition.DefinitionID,
		})
	}
	state.Evidence = values
}

func accumulateEvidence(
	evidence *EvidenceAccumulator,
	score,
	threshold float64,
	elapsed,
	minimum time.Duration,
	at time.Time,
) error {
	if !finite(score) || !finite(threshold) {
		return errors.New("evidence score and threshold must be finite")
	}
	if elapsed <= 0 || minimum <= 0 {
		return errors.New("evidence durations must be positive")
	}
	score = clampFloat(score, 0, 1)
	threshold = clampFloat(threshold, 0, 1)
	capHours := minimum.Hours()
	elapsedHours := elapsed.Hours()
	if score+1e-12 < threshold {
		evidence.PositiveArea = 0
		evidence.ObservedFor = 0
		violation := math.Min(capHours, evidence.ViolationArea.Float64()+(threshold-score)*elapsedHours)
		value, err := NewNonNegative(violation)
		if err != nil {
			return err
		}
		evidence.ViolationArea = value
		evidence.LastUpdatedAt = at
		return nil
	}
	if evidence.ObservedFor == 0 {
		evidence.ViolationArea = 0
	}
	observed := evidence.ObservedFor + elapsed
	if observed > minimum {
		observed = minimum
	}
	positive := math.Min(capHours, evidence.PositiveArea.Float64()+score*elapsedHours)
	violation := math.Min(capHours, evidence.ViolationArea.Float64()+(1-score)*elapsedHours)
	positiveValue, err := NewNonNegative(positive)
	if err != nil {
		return err
	}
	violationValue, err := NewNonNegative(violation)
	if err != nil {
		return err
	}
	evidence.PositiveArea = positiveValue
	evidence.ViolationArea = violationValue
	evidence.ObservedFor = observed
	evidence.LastUpdatedAt = at
	return nil
}

func conditionScore(state AffectiveState, conditions []Condition) (float64, error) {
	if len(conditions) == 0 {
		return 0, errors.New("conditions cannot be empty")
	}
	weighted := 0.0
	total := 0.0
	for _, condition := range conditions {
		value, err := conditionSignalValue(state, condition.Signal)
		if err != nil {
			return 0, err
		}
		weight := condition.Weight.Float64()
		total += weight
		satisfied := false
		switch condition.Operator {
		case ConditionGTE:
			satisfied = value >= condition.Threshold.Float64()
		case ConditionLTE:
			satisfied = value <= condition.Threshold.Float64()
		default:
			return 0, fmt.Errorf("unsupported condition operator %q", condition.Operator)
		}
		if satisfied {
			weighted += weight
		}
	}
	if total <= 0 {
		return 0, errors.New("condition weights must sum to a positive value")
	}
	return clampFloat(weighted/total, 0, 1), nil
}

func conditionSignalValue(state AffectiveState, signal ConditionSignal) (float64, error) {
	switch signal.Layer {
	case SignalEmotion:
		return state.Emotions.Get(Emotion(signal.Name)), nil
	case SignalPhysiology:
		switch signal.Name {
		case "arousal":
			return state.Physiology.Arousal.Float64(), nil
		case "energy":
			return state.Physiology.Energy.Float64(), nil
		case "fatigue":
			return state.Physiology.Fatigue.Float64(), nil
		case "stability":
			return state.Physiology.Stability.Float64(), nil
		case "stress_load":
			return state.Physiology.StressLoad.Float64(), nil
		}
	case SignalRelationship:
		switch signal.Name {
		case "attachment":
			return state.Relationship.Attachment, nil
		case "confidence_in_user":
			return state.Relationship.ConfidenceInUser, nil
		case "openness":
			return state.Relationship.Openness, nil
		case "perceived_safety":
			return state.Relationship.PerceivedSafety, nil
		case "tension":
			return state.Relationship.Tension, nil
		case "unresolved_hurt":
			return state.Relationship.UnresolvedHurt, nil
		}
	case SignalDrive:
		for _, drive := range state.Drives {
			if string(drive.Kind) != signal.Name {
				continue
			}
			switch signal.Field {
			case "level":
				return drive.Level.Float64(), nil
			case "satisfaction":
				return drive.Satisfaction.Float64(), nil
			case "urgency":
				return drive.Urgency.Float64(), nil
			}
		}
	}
	return 0, fmt.Errorf("cannot resolve condition signal %s", signal.String())
}

func complexStateIndex(values []ComplexState, kind ComplexStateKind) int {
	for index, value := range values {
		if value.Kind == kind {
			return index
		}
	}
	return -1
}
