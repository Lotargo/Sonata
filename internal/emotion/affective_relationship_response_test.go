package emotion

import (
	"math"
	"testing"
)

func TestRelationshipResponseRuleVersion(t *testing.T) {
	ruleID, err := relationshipResponseRuleID(relationshipResponseProfileV1)
	if err != nil {
		t.Fatal(err)
	}
	if ruleID != relationshipResponseRuleV1 {
		t.Fatalf("rule ID = %q, want %q", ruleID, relationshipResponseRuleV1)
	}
	if _, err := relationshipResponseRuleID("sonata-affective-unknown"); err == nil {
		t.Fatal("unknown affective profile accepted relationship response rule")
	}
}

func TestRelationshipResponseUsesNeutralSupportPoint(t *testing.T) {
	relationship := RelationshipState{
		Attachment:       0.5,
		Openness:         0.5,
		ConfidenceInUser: 0.5,
		PerceivedSafety:  0.5,
	}
	for _, emotion := range allEmotions {
		modifier, err := relationshipResponse(relationshipResponseProfileV1, relationship, emotion)
		if err != nil {
			t.Fatalf("emotion %s: %v", emotion, err)
		}
		if math.Abs(modifier-1) > 1e-12 {
			t.Fatalf("emotion %s neutral modifier = %f, want 1", emotion, modifier)
		}
	}
}

func TestRelationshipResponseDistinguishesSupportAndStrain(t *testing.T) {
	supported := RelationshipState{
		Attachment:       0.9,
		Openness:         0.9,
		ConfidenceInUser: 0.9,
		PerceivedSafety:  0.9,
		Tension:          0,
		UnresolvedHurt:   0,
	}
	strained := RelationshipState{
		Attachment:       0.1,
		Openness:         0.1,
		ConfidenceInUser: 0.1,
		PerceivedSafety:  0.1,
		Tension:          0.9,
		UnresolvedHurt:   0.9,
	}

	for _, emotion := range []Emotion{EmotionJoy, EmotionTrust, EmotionAnticipation} {
		supportedModifier := mustRelationshipResponse(t, supported, emotion)
		strainedModifier := mustRelationshipResponse(t, strained, emotion)
		if supportedModifier <= strainedModifier {
			t.Fatalf("emotion %s support did not increase response: supported=%f strained=%f", emotion, supportedModifier, strainedModifier)
		}
	}
	for _, emotion := range []Emotion{EmotionFear, EmotionSadness, EmotionDisgust, EmotionAnger} {
		supportedModifier := mustRelationshipResponse(t, supported, emotion)
		strainedModifier := mustRelationshipResponse(t, strained, emotion)
		if strainedModifier <= supportedModifier {
			t.Fatalf("emotion %s strain did not increase response: supported=%f strained=%f", emotion, supportedModifier, strainedModifier)
		}
	}
}

func TestRelationshipResponseClampsExtremeStates(t *testing.T) {
	extremeStrain := RelationshipState{Tension: 1, UnresolvedHurt: 1}
	extremeSupport := RelationshipState{
		Attachment:       1,
		Openness:         1,
		ConfidenceInUser: 1,
		PerceivedSafety:  1,
	}

	if modifier := mustRelationshipResponse(t, extremeStrain, EmotionAnger); modifier != 1.5 {
		t.Fatalf("extreme anger modifier = %f, want 1.5", modifier)
	}
	if modifier := mustRelationshipResponse(t, extremeStrain, EmotionTrust); modifier != 0.5 {
		t.Fatalf("extreme trust modifier = %f, want 0.5", modifier)
	}
	for _, relationship := range []RelationshipState{extremeStrain, extremeSupport} {
		for _, emotion := range allEmotions {
			modifier := mustRelationshipResponse(t, relationship, emotion)
			if modifier < 0.5 || modifier > 1.5 || math.IsNaN(modifier) || math.IsInf(modifier, 0) {
				t.Fatalf("emotion %s modifier out of bounds: %f", emotion, modifier)
			}
		}
	}
}

func mustRelationshipResponse(t *testing.T, relationship RelationshipState, emotion Emotion) float64 {
	t.Helper()
	modifier, err := relationshipResponse(relationshipResponseProfileV1, relationship, emotion)
	if err != nil {
		t.Fatal(err)
	}
	return modifier
}
