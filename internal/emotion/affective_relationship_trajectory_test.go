package emotion

import (
	"reflect"
	"testing"
)

func TestRelationshipChangesSubsequentWarmthResponse(t *testing.T) {
	profile, baseline, start := loadTransitionFixture(t)
	stimulus := Stimulus{
		Kind:       StimulusUserWarmth,
		Source:     "relationship-trajectory",
		Intensity:  0.5,
		Confidence: 1,
		Valence:    0.8,
		Arousal:    0.2,
		CreatedAt:  start,
	}

	supported := baseline.Clone()
	supported.Relationship = supportedRelationship()
	strained := baseline.Clone()
	strained.Relationship = strainedRelationship()

	supportedNext, supportedLog, err := Transition(supported, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	strainedNext, strainedLog, err := Transition(strained, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	supportedJoyDelta := supportedNext.Emotions.Joy - supported.Emotions.Joy
	strainedJoyDelta := strainedNext.Emotions.Joy - strained.Emotions.Joy
	if supportedJoyDelta <= strainedJoyDelta {
		t.Fatalf("support did not increase warmth joy response: supported=%f strained=%f", supportedJoyDelta, strainedJoyDelta)
	}
	supportedTrustDelta := supportedNext.Emotions.Trust - supported.Emotions.Trust
	strainedTrustDelta := strainedNext.Emotions.Trust - strained.Emotions.Trust
	if supportedTrustDelta <= strainedTrustDelta {
		t.Fatalf("support did not increase warmth trust response: supported=%f strained=%f", supportedTrustDelta, strainedTrustDelta)
	}
	if supportedLog.RelationshipRule != relationshipResponseRuleV1 || strainedLog.RelationshipRule != relationshipResponseRuleV1 {
		t.Fatalf("unexpected relationship rules: supported=%q strained=%q", supportedLog.RelationshipRule, strainedLog.RelationshipRule)
	}

	replayed, replayLog, err := Transition(supported, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(supportedNext, replayed) || !reflect.DeepEqual(supportedLog, replayLog) {
		t.Fatal("relationship trajectory replay produced a different state or log")
	}
}

func TestRelationshipChangesSubsequentHostilityResponse(t *testing.T) {
	profile, baseline, start := loadTransitionFixture(t)
	stimulus := Stimulus{
		Kind:       StimulusUserHostility,
		Source:     "relationship-trajectory",
		Intensity:  0.5,
		Confidence: 1,
		Valence:    -1,
		Arousal:    0.8,
		CreatedAt:  start,
	}

	supported := baseline.Clone()
	supported.Relationship = supportedRelationship()
	strained := baseline.Clone()
	strained.Relationship = strainedRelationship()

	supportedNext, _, err := Transition(supported, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}
	strainedNext, _, err := Transition(strained, []Stimulus{stimulus}, start, profile)
	if err != nil {
		t.Fatal(err)
	}

	supportedAngerDelta := supportedNext.Emotions.Anger - supported.Emotions.Anger
	strainedAngerDelta := strainedNext.Emotions.Anger - strained.Emotions.Anger
	if supportedAngerDelta >= strainedAngerDelta {
		t.Fatalf("support did not reduce hostility anger response: supported=%f strained=%f", supportedAngerDelta, strainedAngerDelta)
	}
	supportedDisgustDelta := supportedNext.Emotions.Disgust - supported.Emotions.Disgust
	strainedDisgustDelta := strainedNext.Emotions.Disgust - strained.Emotions.Disgust
	if supportedDisgustDelta >= strainedDisgustDelta {
		t.Fatalf("support did not reduce hostility disgust response: supported=%f strained=%f", supportedDisgustDelta, strainedDisgustDelta)
	}
	supportedTrustLoss := supported.Emotions.Trust - supportedNext.Emotions.Trust
	strainedTrustLoss := strained.Emotions.Trust - strainedNext.Emotions.Trust
	if supportedTrustLoss >= strainedTrustLoss {
		t.Fatalf("support did not buffer hostility trust loss: supported=%f strained=%f", supportedTrustLoss, strainedTrustLoss)
	}
}

func TestCurrentStimulusRelationshipEffectsDoNotBootstrapResponse(t *testing.T) {
	profile, baseline, start := loadTransitionFixture(t)
	definition, exists := profile.Stimuli.Definition(StimulusUserWarmth)
	if !exists {
		t.Fatal("warmth definition not found")
	}
	withoutRelationshipEffects := definition
	withoutRelationshipEffects.RelationshipEffects = nil
	stimulus := Stimulus{
		Kind:       StimulusUserWarmth,
		Source:     "relationship-ordering",
		Intensity:  0.5,
		Confidence: 1,
		Valence:    0.8,
		Arousal:    0.2,
		CreatedAt:  start,
	}

	withEffects := baseline.Clone()
	withoutEffects := baseline.Clone()
	if err := applyAffectiveStimulus(&withEffects, stimulus, definition, profile.Dynamics); err != nil {
		t.Fatal(err)
	}
	if err := applyAffectiveStimulus(&withoutEffects, stimulus, withoutRelationshipEffects, profile.Dynamics); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(withEffects.Emotions, withoutEffects.Emotions) {
		t.Fatalf("current stimulus relationship effects changed its own emotional response: with=%#v without=%#v", withEffects.Emotions, withoutEffects.Emotions)
	}
	if reflect.DeepEqual(withEffects.Relationship, withoutEffects.Relationship) {
		t.Fatal("test setup did not produce different relationship updates")
	}
}

func supportedRelationship() RelationshipState {
	return RelationshipState{
		Attachment:       0.9,
		Openness:         0.9,
		ConfidenceInUser: 0.9,
		PerceivedSafety:  0.9,
	}
}

func strainedRelationship() RelationshipState {
	return RelationshipState{
		Attachment:       0.1,
		Openness:         0.1,
		ConfidenceInUser: 0.1,
		PerceivedSafety:  0.1,
		Tension:          0.9,
		UnresolvedHurt:   0.9,
	}
}
