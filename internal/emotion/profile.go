package emotion

import (
	"errors"
	"fmt"

	"github.com/Lotargo/Sonata/internal/config"
)

type Profile struct {
	IdentityID            string
	Baseline              Vector
	DecayRates            Vector
	RelationshipBaseline  RelationshipState
	RelationshipDecayRate float64
	FatigueDecayRate      float64
	StabilityDecayRate    float64
	MaxDelta              float64
	OppositionSuppression float64
	DominanceCeiling      float64
}

func NewProfileFromConfig(identityID string, value config.EmotionConfig) (Profile, error) {
	baseline, err := VectorFromMap(value.Baseline)
	if err != nil {
		return Profile{}, fmt.Errorf("load emotion baseline: %w", err)
	}
	decayRates, err := vectorFromNonNegativeMap(value.DecayRates)
	if err != nil {
		return Profile{}, fmt.Errorf("load emotion decay rates: %w", err)
	}
	profile := Profile{
		IdentityID: identityID,
		Baseline:   baseline,
		DecayRates: decayRates,
		RelationshipBaseline: RelationshipState{
			Attachment:       0,
			Openness:         0.5,
			Tension:          0,
			ConfidenceInUser: 0.5,
			PerceivedSafety:  0.5,
			UnresolvedHurt:   0,
		},
		RelationshipDecayRate: 0.01,
		FatigueDecayRate:      0.08,
		StabilityDecayRate:    0.04,
		MaxDelta:              value.MaxDelta,
		OppositionSuppression: 0.35,
		DominanceCeiling:      1.4,
	}
	if !value.Relationship.PerUser || value.Relationship.Global {
		return Profile{}, errors.New("mini MVP emotion profile requires per-user state and forbids global state")
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (profile Profile) Validate() error {
	if (StateKey{IdentityID: profile.IdentityID, UserID: "validation"}).Validate() != nil {
		return errors.New("emotion profile identity ID is required")
	}
	if err := profile.Baseline.Validate(); err != nil {
		return fmt.Errorf("invalid baseline: %w", err)
	}
	for _, emotion := range allEmotions {
		if profile.DecayRates.Get(emotion) < 0 {
			return fmt.Errorf("decay rate for %s cannot be negative", emotion)
		}
	}
	if err := profile.RelationshipBaseline.Validate(); err != nil {
		return fmt.Errorf("invalid relationship baseline: %w", err)
	}
	if profile.RelationshipDecayRate < 0 || profile.FatigueDecayRate < 0 || profile.StabilityDecayRate < 0 {
		return errors.New("decay rates cannot be negative")
	}
	if profile.MaxDelta <= 0 || profile.MaxDelta > 1 {
		return errors.New("max delta must be greater than 0 and at most 1")
	}
	if profile.OppositionSuppression < 0 || profile.OppositionSuppression > 1 {
		return errors.New("opposition suppression must be between 0 and 1")
	}
	if profile.DominanceCeiling <= 1 || profile.DominanceCeiling > 2 {
		return errors.New("dominance ceiling must be greater than 1 and at most 2")
	}
	return nil
}

func vectorFromNonNegativeMap(values map[string]float64) (Vector, error) {
	if len(values) != len(allEmotions) {
		return Vector{}, fmt.Errorf("emotion vector requires exactly %d values", len(allEmotions))
	}
	var vector Vector
	for _, emotion := range allEmotions {
		value, exists := values[string(emotion)]
		if !exists {
			return Vector{}, fmt.Errorf("emotion vector is missing %q", emotion)
		}
		if value < 0 {
			return Vector{}, fmt.Errorf("emotion %s cannot be negative", emotion)
		}
		vector.Set(emotion, value)
	}
	for name := range values {
		if !Emotion(name).Valid() {
			return Vector{}, fmt.Errorf("emotion vector contains unknown emotion %q", name)
		}
	}
	return vector, nil
}
