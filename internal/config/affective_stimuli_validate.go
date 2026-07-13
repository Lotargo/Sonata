package config

import (
	"fmt"
	"sort"
	"strings"
)

var affectiveStimulusOrder = []string{
	"conversation_break",
	"conversation_return",
	"promise_broken",
	"promise_kept",
	"response_appreciated",
	"response_rejected",
	"tool_failure",
	"tool_success",
	"user_apology",
	"user_boundary",
	"user_distress",
	"user_hostility",
	"user_rejection",
	"user_success",
	"user_trust",
	"user_warmth",
}

func (config EmotionConfig) ValidateAffectiveStimuli() []string {
	var problems []string
	validateExactKeys(&problems, "emotion.affective.stimuli", config.Affective.Stimuli, affectiveStimulusOrder)
	definitionIDs := make(map[string]string, len(config.Affective.Stimuli))
	for _, name := range affectiveStimulusOrder {
		definition, exists := config.Affective.Stimuli[name]
		if !exists {
			continue
		}
		prefix := "emotion.affective.stimuli." + name
		validateDefinitionID(&problems, prefix, definition.DefinitionID, definitionIDs)
		if len(definition.EmotionEffects)+len(definition.RelationshipEffects)+len(definition.PhysiologyEffects) == 0 {
			problems = append(problems, prefix+" must contain at least one effect")
		}
		validateSubsetFloatMap(&problems, prefix+".emotion_effects", definition.EmotionEffects, affectiveEmotionNames, validateSignedUnit)
		validateSubsetFloatMap(&problems, prefix+".relationship_effects", definition.RelationshipEffects,
			[]string{"attachment", "confidence_in_user", "openness", "perceived_safety", "tension", "unresolved_hurt"}, validateSignedUnit)
		validateSubsetFloatMap(&problems, prefix+".physiology_effects", definition.PhysiologyEffects,
			[]string{"arousal", "energy", "fatigue", "stability", "stress_load"}, validateSignedUnit)
		if strings.TrimSpace(definition.DefinitionID) != definition.DefinitionID {
			problems = append(problems, fmt.Sprintf("%s.definition_id cannot contain surrounding whitespace", prefix))
		}
	}
	sort.Strings(problems)
	return problems
}
