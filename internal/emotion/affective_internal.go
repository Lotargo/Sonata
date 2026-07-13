package emotion

import (
	"math"
	"time"
)

func advancePhysiology(state *AffectiveState, elapsed time.Duration, profile AffectiveRuntimeProfile) {
	if elapsed <= 0 {
		return
	}
	hours := elapsed.Hours()
	for _, recovery := range profile.Initial.PhysiologyRecovery {
		current := physiologyValue(state.Physiology, recovery.Name)
		target := physiologyValue(profile.Initial.Physiology, recovery.Name)
		rate := recovery.Value.Float64() * (0.5 + profile.Dynamics.Personality.RecoveryCapacity.Float64())
		rate *= activeNamedRecoveryMultiplier(*state, profile.Dynamics, recovery.Name, true)
		value := target + (current-target)*math.Exp(-rate*hours)
		setPhysiologyValue(&state.Physiology, recovery.Name, value)
	}
}

func advanceDrives(state *AffectiveState, elapsed time.Duration, profile AffectiveProfile) {
	if elapsed <= 0 {
		return
	}
	hours := elapsed.Hours()
	for index := range state.Drives {
		if index >= len(profile.Drives) || state.Drives[index].Kind != profile.Drives[index].Kind {
			continue
		}
		definition := profile.Drives[index]
		target := driveUrgencyTarget(*state, profile, state.Drives[index])
		rate := definition.GrowthRate.Float64()
		current := state.Drives[index].Urgency.Float64()
		value := target + (current-target)*math.Exp(-rate*hours)
		state.Drives[index].Urgency = Unit(clampFloat(value, 0, 1))
	}
}

func applyDrivePressure(state *AffectiveState, elapsed time.Duration, profile AffectiveProfile) {
	if elapsed <= 0 {
		return
	}
	hours := elapsed.Hours()
	var deltas [len(allEmotions)]float64
	for index, drive := range state.Drives {
		if index >= len(profile.Drives) || drive.Kind != profile.Drives[index].Kind {
			continue
		}
		definition := profile.Drives[index]
		pressure := drive.Urgency.Float64() * definition.GrowthRate.Float64() * hours
		for _, effect := range definition.EmotionEffects {
			emotionIndex := emotionIndex(effect.Emotion)
			dynamics := profile.Dynamics[emotionIndex]
			delta := effect.Weight.Float64() * pressure
			if delta >= 0 {
				delta = math.Min(delta, dynamics.MaxPositiveDelta.Float64())
			} else {
				delta = math.Max(delta, -dynamics.MaxNegativeDelta.Float64())
			}
			deltas[emotionIndex] += delta
		}
	}
	applyEmotionDelta(state, deltas, profile)
}

func driveUrgencyTarget(state AffectiveState, profile AffectiveProfile, drive DriveState) float64 {
	target := drive.Level.Float64() * (1 - drive.Satisfaction.Float64())
	target *= activeDriveUrgencyMultiplier(state, profile, drive.Kind)
	return clampFloat(target, 0, 1)
}

func activeNamedRecoveryMultiplier(state AffectiveState, profile AffectiveProfile, name string, physiology bool) float64 {
	result := 1.0
	for _, active := range state.ComplexStates {
		if !active.Active() {
			continue
		}
		definition, exists := complexDefinition(profile, active.Kind)
		if !exists || definition.DefinitionID != active.DefinitionID {
			continue
		}
		values := definition.Effects.RelationshipRecovery
		if physiology {
			values = definition.Effects.PhysiologyRecovery
		}
		for _, modifier := range values {
			if modifier.Name == name {
				result *= interpolateMultiplier(modifier.Value.Float64(), active.Activation.Float64())
			}
		}
	}
	return clampFloat(result, 0, 2)
}

func physiologyValue(value Physiology, name string) float64 {
	switch name {
	case "arousal":
		return value.Arousal.Float64()
	case "energy":
		return value.Energy.Float64()
	case "fatigue":
		return value.Fatigue.Float64()
	case "stability":
		return value.Stability.Float64()
	case "stress_load":
		return value.StressLoad.Float64()
	default:
		return 0
	}
}

func setPhysiologyValue(value *Physiology, name string, next float64) {
	next = clampFloat(next, 0, 1)
	switch name {
	case "arousal":
		value.Arousal = Unit(next)
	case "energy":
		value.Energy = Unit(next)
	case "fatigue":
		value.Fatigue = Unit(next)
	case "stability":
		value.Stability = Unit(next)
	case "stress_load":
		value.StressLoad = Unit(next)
	}
}
