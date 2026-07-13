package config

import (
	"fmt"
	"sort"
)

func (config EmotionConfig) ValidateAffectiveInitial() []string {
	var problems []string
	physiology := config.Affective.InitialPhysiology
	fields := map[string]float64{
		"arousal":     physiology.Arousal,
		"energy":      physiology.Energy,
		"fatigue":     physiology.Fatigue,
		"stability":   physiology.Stability,
		"stress_load": physiology.StressLoad,
	}
	for name, value := range fields {
		if !validateUnit(value) {
			problems = append(problems, fmt.Sprintf("emotion.affective.initial_physiology.%s must be finite and between 0 and 1", name))
		}
	}
	if physiology.Energy <= 0 {
		problems = append(problems, "emotion.affective.initial_physiology.energy must be greater than 0")
	}
	if physiology.Stability <= 0 {
		problems = append(problems, "emotion.affective.initial_physiology.stability must be greater than 0")
	}
	for _, name := range affectiveDriveNames {
		definition, exists := config.Affective.Drives[name]
		if !exists {
			continue
		}
		if !validatePositiveUnit(definition.InitialSatisfaction) {
			problems = append(problems, fmt.Sprintf("emotion.affective.drives.%s.initial_satisfaction must be finite, greater than 0 and at most 1", name))
		}
	}
	sort.Strings(problems)
	return problems
}
