package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryAffectiveStimuliAndInitialStateValidate(t *testing.T) {
	setRepositoryConfigSecrets(t)
	root := filepath.Join("..", "..", "config")
	cfg, err := NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatalf("load repository config: %v", err)
	}
	if problems := cfg.Emotion.ValidateAffectiveStimuli(); len(problems) != 0 {
		t.Fatalf("valid stimulus profile returned problems: %v", problems)
	}
	if problems := cfg.Emotion.ValidateAffectiveInitial(); len(problems) != 0 {
		t.Fatalf("valid initial profile returned problems: %v", problems)
	}
	if len(cfg.Emotion.Affective.Stimuli) != len(affectiveStimulusOrder) {
		t.Fatalf("stimulus definitions = %d, want %d", len(cfg.Emotion.Affective.Stimuli), len(affectiveStimulusOrder))
	}
}

func TestAffectiveRuntimeValidationRejectsMissingStimulusAndInitialSatisfaction(t *testing.T) {
	setRepositoryConfigSecrets(t)
	root := filepath.Join("..", "..", "config")
	cfg, err := NewLoader(nil).Load(context.Background(), root, "local")
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Emotion.Affective.Stimuli, "user_hostility")
	recovery := cfg.Emotion.Affective.Drives["recovery"]
	recovery.InitialSatisfaction = 0
	cfg.Emotion.Affective.Drives["recovery"] = recovery

	stimulusProblems := strings.Join(cfg.Emotion.ValidateAffectiveStimuli(), "; ")
	if !strings.Contains(stimulusProblems, "stimuli is missing user_hostility") {
		t.Fatalf("missing stimulus was not rejected: %s", stimulusProblems)
	}
	initialProblems := strings.Join(cfg.Emotion.ValidateAffectiveInitial(), "; ")
	if !strings.Contains(initialProblems, "recovery.initial_satisfaction") {
		t.Fatalf("missing initial satisfaction was not rejected: %s", initialProblems)
	}
}
