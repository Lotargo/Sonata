package emotion

import (
	"fmt"
	"strings"
)

const (
	relationshipResponseRuleV1    = "relationship-response-v1"
	relationshipResponseProfileV1 = "sonata-affective-v1.0.0"
)

type relationshipResponseWeights struct {
	Support SignedUnit
	Strain  SignedUnit
}

func relationshipResponseRuleID(profileVersion string) (string, error) {
	switch strings.TrimSpace(profileVersion) {
	case relationshipResponseProfileV1:
		return relationshipResponseRuleV1, nil
	default:
		return "", fmt.Errorf("unsupported relationship response profile %q", profileVersion)
	}
}

func relationshipResponse(
	profileVersion string,
	relationship RelationshipState,
	emotion Emotion,
	direction float64,
) (float64, error) {
	if _, err := relationshipResponseRuleID(profileVersion); err != nil {
		return 0, err
	}
	if !emotion.Valid() {
		return 0, fmt.Errorf("invalid relationship response emotion %q", emotion)
	}
	if direction == 0 {
		return 1, nil
	}

	weights, ok := relationshipResponseWeightsV1(emotion)
	if !ok {
		return 0, fmt.Errorf("missing relationship response weights for %s", emotion)
	}

	support := clampFloat(
		(relationship.Attachment+
			relationship.Openness+
			relationship.ConfidenceInUser+
			relationship.PerceivedSafety)/4,
		0,
		1,
	)
	strain := clampFloat(
		0.60*relationship.Tension+0.40*relationship.UnresolvedHurt,
		0,
		1,
	)

	modifier := 1.0
	modifier += weights.Support.Float64() * (support - 0.50)
	modifier += weights.Strain.Float64() * strain
	modifier = clampFloat(modifier, 0.50, 1.50)
	if direction < 0 {
		modifier = 2 - modifier
	}
	return clampFloat(modifier, 0.50, 1.50), nil
}

func relationshipResponseWeightsV1(emotion Emotion) (relationshipResponseWeights, bool) {
	switch emotion {
	case EmotionJoy:
		return relationshipResponseWeights{Support: 0.30, Strain: -0.20}, true
	case EmotionTrust:
		return relationshipResponseWeights{Support: 0.45, Strain: -0.45}, true
	case EmotionFear:
		return relationshipResponseWeights{Support: -0.25, Strain: 0.40}, true
	case EmotionSurprise:
		return relationshipResponseWeights{Support: 0.05, Strain: 0.00}, true
	case EmotionSadness:
		return relationshipResponseWeights{Support: -0.20, Strain: 0.30}, true
	case EmotionDisgust:
		return relationshipResponseWeights{Support: -0.15, Strain: 0.35}, true
	case EmotionAnger:
		return relationshipResponseWeights{Support: -0.20, Strain: 0.45}, true
	case EmotionAnticipation:
		return relationshipResponseWeights{Support: 0.15, Strain: -0.10}, true
	default:
		return relationshipResponseWeights{}, false
	}
}
