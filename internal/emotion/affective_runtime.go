package emotion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AffectiveRuntime owns one validated v1 profile and advances owner-scoped
// state for incoming user messages. It performs no LLM, tool or network calls.
type AffectiveRuntime struct {
	identityID   string
	profile      AffectiveRuntimeProfile
	relationship RelationshipState
	store        AffectiveStateStore
	extractor    Extractor
	clock        Clock
}

func NewAffectiveRuntime(
	identityID string,
	profile AffectiveRuntimeProfile,
	store AffectiveStateStore,
	clock Clock,
) (*AffectiveRuntime, error) {
	identityID = strings.TrimSpace(identityID)
	if err := (StateKey{IdentityID: identityID, UserID: "validation"}).Validate(); err != nil {
		return nil, errors.New("affective runtime identity ID is required")
	}
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("validate affective runtime profile: %w", err)
	}
	if store == nil {
		return nil, errors.New("affective state store is required")
	}
	if clock == nil {
		clock = time.Now
	}
	relationship := BaselineRelationshipState()
	if err := relationship.Validate(); err != nil {
		return nil, fmt.Errorf("validate affective relationship baseline: %w", err)
	}
	return &AffectiveRuntime{
		identityID:   identityID,
		profile:      profile,
		relationship: relationship,
		store:        store,
		extractor:    Extractor{},
		clock:        clock,
	}, nil
}

func BaselineRelationshipState() RelationshipState {
	return RelationshipState{
		Openness:         0.5,
		ConfidenceInUser: 0.5,
		PerceivedSafety:  0.5,
	}
}

func (runtime *AffectiveRuntime) Key(userID string) StateKey {
	if runtime == nil {
		return StateKey{UserID: strings.TrimSpace(userID)}
	}
	return StateKey{IdentityID: runtime.identityID, UserID: strings.TrimSpace(userID)}
}

func (runtime *AffectiveRuntime) ProcessUserMessage(ctx context.Context, userID, text string) (AffectiveReport, error) {
	if runtime == nil || runtime.store == nil || runtime.clock == nil {
		return AffectiveReport{}, errors.New("affective runtime is not initialized")
	}
	key := runtime.Key(userID)
	if err := key.Validate(); err != nil {
		return AffectiveReport{}, err
	}

	for attempt := 0; attempt < maxMutationAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return AffectiveReport{}, err
		}
		previous, exists, err := runtime.store.Load(ctx, key)
		if err != nil {
			return AffectiveReport{}, err
		}
		now := runtime.clock().UTC()
		if exists && now.Before(previous.LastUpdatedAt) {
			now = previous.LastUpdatedAt
		}
		if !exists {
			previous, err = runtime.baselineState(key, now)
			if err != nil {
				return AffectiveReport{}, err
			}
		}

		stimuli := runtime.extractor.ExtractUserMessage(text, now)
		next, _, err := Transition(previous, stimuli, now, runtime.profile)
		if err != nil {
			return AffectiveReport{}, err
		}
		if next.Version == previous.Version {
			return buildAffectiveReport(previous, StatusHealthy, now)
		}
		if err := runtime.store.CompareAndSwap(ctx, key, previous.Version, next); err != nil {
			if errors.Is(err, ErrVersionConflict) {
				continue
			}
			return AffectiveReport{}, err
		}
		return buildAffectiveReport(next, StatusHealthy, now)
	}
	return AffectiveReport{}, ErrVersionConflict
}

// DegradedReport builds a safe baseline projection without reading or writing
// canonical state. Callers use it only after a runtime or storage failure.
func (runtime *AffectiveRuntime) DegradedReport(userID string) (AffectiveReport, error) {
	if runtime == nil || runtime.clock == nil {
		return AffectiveReport{}, errors.New("affective runtime is not initialized")
	}
	key := runtime.Key(userID)
	if err := key.Validate(); err != nil {
		return AffectiveReport{}, err
	}
	now := runtime.clock().UTC()
	state, err := runtime.baselineState(key, now)
	if err != nil {
		return AffectiveReport{}, err
	}
	return buildAffectiveReport(state, StatusDegraded, now)
}

func (runtime *AffectiveRuntime) baselineState(key StateKey, at time.Time) (AffectiveState, error) {
	return NewBaselineAffectiveStateFromProfiles(
		key,
		runtime.profile.Dynamics,
		runtime.profile.Initial,
		runtime.relationship,
		at,
	)
}
