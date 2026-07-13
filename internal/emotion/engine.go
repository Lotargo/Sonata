package emotion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const maxMutationAttempts = 128

type Clock func() time.Time

type Engine struct {
	profile Profile
	store   Store
	clock   Clock
}

func NewEngine(profile Profile, store Store, clock Clock) (*Engine, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, errors.New("emotion store is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Engine{profile: profile, store: store, clock: clock}, nil
}

func (engine *Engine) Key(userID string) StateKey {
	return StateKey{IdentityID: engine.profile.IdentityID, UserID: userID}
}

func (engine *Engine) ApplyStimuli(ctx context.Context, key StateKey, stimuli []Stimulus) (Report, error) {
	if err := engine.validateKey(key); err != nil {
		return Report{}, err
	}
	for index, stimulus := range stimuli {
		if err := stimulus.Validate(); err != nil {
			return Report{}, fmt.Errorf("validate stimulus %d: %w", index, err)
		}
	}
	return engine.mutate(ctx, key, stimuli, true)
}

func (engine *Engine) GetReport(ctx context.Context, key StateKey) (Report, error) {
	if err := engine.validateKey(key); err != nil {
		return Report{}, err
	}
	return engine.mutate(ctx, key, nil, false)
}

func (engine *Engine) GetReportOrBaseline(ctx context.Context, key StateKey) Report {
	report, err := engine.GetReport(ctx, key)
	if err == nil {
		return report
	}
	fallback := engine.baselineReport(key, engine.clock())
	fallback.Status = StatusDegraded
	fallback.Text = compactText(fallback)
	return fallback
}

func (engine *Engine) mutate(ctx context.Context, key StateKey, stimuli []Stimulus, forceWrite bool) (Report, error) {
	for attempt := 0; attempt < maxMutationAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		now := engine.clock().UTC()
		state, exists, err := engine.store.Load(ctx, key)
		if err != nil {
			return Report{}, err
		}
		if !exists {
			state = engine.baselineState(key, now)
		}
		expectedVersion := state.Version
		changed := applyDecay(&state, engine.profile, now)
		for _, stimulus := range stimuli {
			engine.applyStimulus(&state, stimulus)
			changed = true
		}
		if !changed && !forceWrite {
			return buildReport(state, StatusHealthy, now), nil
		}
		state.LastUpdatedAt = now
		state.Version = expectedVersion + 1
		if err := engine.store.CompareAndSwap(ctx, key, expectedVersion, state); err != nil {
			if errors.Is(err, ErrVersionConflict) {
				continue
			}
			return Report{}, err
		}
		return buildReport(state, StatusHealthy, now), nil
	}
	return Report{}, ErrVersionConflict
}

func (engine *Engine) validateKey(key StateKey) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if key.IdentityID != engine.profile.IdentityID {
		return errors.New("emotion state identity does not match engine profile")
	}
	return nil
}

func (engine *Engine) baselineState(key StateKey, now time.Time) State {
	return State{
		Key:           key,
		Emotions:      engine.profile.Baseline,
		Relationship:  engine.profile.RelationshipBaseline,
		Fatigue:       0,
		Stability:     1,
		LastUpdatedAt: now,
		Version:       0,
	}
}

func (engine *Engine) baselineReport(key StateKey, now time.Time) Report {
	return buildReport(engine.baselineState(key, now), StatusHealthy, now)
}

type transitionEffect struct {
	emotions     map[Emotion]float64
	relationship RelationshipState
	fatigue      float64
	stability    float64
}

var transitionEffects = map[StimulusKind]transitionEffect{
	StimulusUserWarmth: {
		emotions:     map[Emotion]float64{Joy: 0.12, Trust: 0.10},
		relationship: RelationshipState{Attachment: 0.04, Openness: 0.04, Tension: -0.05, PerceivedSafety: 0.04},
	},
	StimulusUserHostility: {
		emotions:     map[Emotion]float64{Anger: 0.18, Disgust: 0.12, Fear: 0.06, Trust: -0.15},
		relationship: RelationshipState{Tension: 0.15, PerceivedSafety: -0.12, UnresolvedHurt: 0.10},
		stability:    -0.05,
	},
	StimulusUserTrust: {
		emotions:     map[Emotion]float64{Trust: 0.15, Joy: 0.03},
		relationship: RelationshipState{Attachment: 0.05, Openness: 0.06, ConfidenceInUser: 0.08, PerceivedSafety: 0.05},
	},
	StimulusUserRejection: {
		emotions:     map[Emotion]float64{Sadness: 0.12, Fear: 0.05, Trust: -0.05},
		relationship: RelationshipState{Attachment: -0.12, Openness: -0.10, Tension: 0.08, UnresolvedHurt: 0.10},
	},
	StimulusUserDistress: {
		emotions:     map[Emotion]float64{Sadness: 0.10, Fear: 0.12, Trust: 0.02},
		relationship: RelationshipState{Attachment: 0.03, Openness: 0.02},
	},
	StimulusUserSuccess: {
		emotions:     map[Emotion]float64{Joy: 0.16, Anticipation: 0.08},
		relationship: RelationshipState{ConfidenceInUser: 0.07},
	},
	StimulusUserApology: {
		emotions:     map[Emotion]float64{Trust: 0.10, Joy: 0.04, Anger: -0.08},
		relationship: RelationshipState{Tension: -0.12, UnresolvedHurt: -0.10, Openness: 0.04},
	},
	StimulusUserBoundary: {
		emotions:     map[Emotion]float64{Trust: 0.02},
		relationship: RelationshipState{Attachment: -0.08, Openness: -0.08, Tension: -0.03},
	},
	StimulusConversationReturn: {
		emotions:     map[Emotion]float64{Joy: 0.05, Trust: 0.03},
		relationship: RelationshipState{Attachment: 0.03},
	},
	StimulusConversationBreak: {
		emotions:     map[Emotion]float64{Sadness: 0.04},
		relationship: RelationshipState{Attachment: -0.02},
	},
	StimulusPromiseKept: {
		emotions:     map[Emotion]float64{Trust: 0.18, Joy: 0.04},
		relationship: RelationshipState{ConfidenceInUser: 0.12, PerceivedSafety: 0.06},
	},
	StimulusPromiseBroken: {
		emotions:     map[Emotion]float64{Anger: 0.08, Sadness: 0.08, Trust: -0.18},
		relationship: RelationshipState{Tension: 0.10, UnresolvedHurt: 0.12, ConfidenceInUser: -0.12},
	},
	StimulusToolSuccess: {
		emotions:  map[Emotion]float64{Joy: 0.05, Anticipation: 0.04},
		stability: 0.03,
	},
	StimulusToolFailure: {
		emotions:  map[Emotion]float64{Sadness: 0.04, Anger: 0.03},
		fatigue:   0.05,
		stability: -0.04,
	},
	StimulusResponseRejected: {
		emotions:     map[Emotion]float64{Sadness: 0.08, Anger: 0.02},
		relationship: RelationshipState{Tension: 0.05, Openness: -0.04},
	},
	StimulusResponseAppreciated: {
		emotions:     map[Emotion]float64{Joy: 0.10, Trust: 0.08},
		relationship: RelationshipState{Attachment: 0.03, Openness: 0.03},
	},
}

var opposites = map[Emotion]Emotion{
	Joy: Sadness, Sadness: Joy,
	Trust: Disgust, Disgust: Trust,
	Fear: Anger, Anger: Fear,
	Surprise: Anticipation, Anticipation: Surprise,
}

func (engine *Engine) applyStimulus(state *State, stimulus Stimulus) {
	effect := transitionEffects[stimulus.Kind]
	scale := stimulus.Intensity * stimulus.Confidence
	for emotion, weight := range effect.emotions {
		delta := clampSigned(weight*scale, engine.profile.MaxDelta)
		state.Emotions.Set(emotion, clamp01(state.Emotions.Get(emotion)+delta))
		if delta > 0 {
			opposite := opposites[emotion]
			suppression := math.Min(delta*engine.profile.OppositionSuppression, engine.profile.MaxDelta)
			state.Emotions.Set(opposite, clamp01(state.Emotions.Get(opposite)-suppression))
		}
	}
	applyRelationshipDelta(&state.Relationship, effect.relationship, scale, engine.profile.MaxDelta)
	state.Fatigue = clamp01(state.Fatigue + clampSigned(effect.fatigue*scale, engine.profile.MaxDelta))
	state.Stability = clamp01(state.Stability + clampSigned(effect.stability*scale, engine.profile.MaxDelta))
	applyDominance(&state.Emotions, engine.profile.DominanceCeiling)
}

func applyRelationshipDelta(state *RelationshipState, delta RelationshipState, scale, maxDelta float64) {
	state.Attachment = clamp01(state.Attachment + clampSigned(delta.Attachment*scale, maxDelta))
	state.Openness = clamp01(state.Openness + clampSigned(delta.Openness*scale, maxDelta))
	state.Tension = clamp01(state.Tension + clampSigned(delta.Tension*scale, maxDelta))
	state.ConfidenceInUser = clamp01(state.ConfidenceInUser + clampSigned(delta.ConfidenceInUser*scale, maxDelta))
	state.PerceivedSafety = clamp01(state.PerceivedSafety + clampSigned(delta.PerceivedSafety*scale, maxDelta))
	state.UnresolvedHurt = clamp01(state.UnresolvedHurt + clampSigned(delta.UnresolvedHurt*scale, maxDelta))
}

func applyDominance(vector *Vector, ceiling float64) {
	pairs := [][2]Emotion{{Joy, Sadness}, {Trust, Disgust}, {Fear, Anger}, {Surprise, Anticipation}}
	for _, pair := range pairs {
		left := vector.Get(pair[0])
		right := vector.Get(pair[1])
		excess := left + right - ceiling
		if excess <= 0 {
			continue
		}
		if left >= right {
			vector.Set(pair[1], clamp01(right-excess))
		} else {
			vector.Set(pair[0], clamp01(left-excess))
		}
	}
}

func applyDecay(state *State, profile Profile, now time.Time) bool {
	if state.LastUpdatedAt.IsZero() || !now.After(state.LastUpdatedAt) {
		return false
	}
	hours := now.Sub(state.LastUpdatedAt).Hours()
	for _, emotion := range allEmotions {
		state.Emotions.Set(emotion, decayValue(state.Emotions.Get(emotion), profile.Baseline.Get(emotion), profile.DecayRates.Get(emotion), hours))
	}
	state.Relationship.Attachment = decayValue(state.Relationship.Attachment, profile.RelationshipBaseline.Attachment, profile.RelationshipDecayRate, hours)
	state.Relationship.Openness = decayValue(state.Relationship.Openness, profile.RelationshipBaseline.Openness, profile.RelationshipDecayRate, hours)
	state.Relationship.Tension = decayValue(state.Relationship.Tension, profile.RelationshipBaseline.Tension, profile.RelationshipDecayRate, hours)
	state.Relationship.ConfidenceInUser = decayValue(state.Relationship.ConfidenceInUser, profile.RelationshipBaseline.ConfidenceInUser, profile.RelationshipDecayRate, hours)
	state.Relationship.PerceivedSafety = decayValue(state.Relationship.PerceivedSafety, profile.RelationshipBaseline.PerceivedSafety, profile.RelationshipDecayRate, hours)
	state.Relationship.UnresolvedHurt = decayValue(state.Relationship.UnresolvedHurt, profile.RelationshipBaseline.UnresolvedHurt, profile.RelationshipDecayRate, hours)
	state.Fatigue = decayValue(state.Fatigue, 0, profile.FatigueDecayRate, hours)
	state.Stability = decayValue(state.Stability, 1, profile.StabilityDecayRate, hours)
	state.LastUpdatedAt = now
	return true
}

func decayValue(current, baseline, rate, hours float64) float64 {
	if rate == 0 || hours <= 0 {
		return current
	}
	return clamp01(baseline + (current-baseline)*math.Exp(-rate*hours))
}

func buildReport(state State, status Status, now time.Time) Report {
	signals := make([]EmotionSignal, 0, len(allEmotions))
	for _, emotion := range allEmotions {
		signals = append(signals, EmotionSignal{Name: emotion, Value: state.Emotions.Get(emotion)})
	}
	sort.SliceStable(signals, func(left, right int) bool {
		if signals[left].Value == signals[right].Value {
			return signals[left].Name < signals[right].Name
		}
		return signals[left].Value > signals[right].Value
	})
	if len(signals) > 3 {
		signals = signals[:3]
	}
	report := Report{
		Status:           status,
		StateVersion:     state.Version,
		DominantEmotions: signals,
		Relationship: RelationshipReport{
			Attachment:       state.Relationship.Attachment,
			Openness:         state.Relationship.Openness,
			Tension:          state.Relationship.Tension,
			ConfidenceInUser: state.Relationship.ConfidenceInUser,
			PerceivedSafety:  state.Relationship.PerceivedSafety,
			UnresolvedHurt:   state.Relationship.UnresolvedHurt,
		},
		Fatigue:           state.Fatigue,
		Stability:         state.Stability,
		ToneBias:          toneBias(state),
		RiskSensitivity:   clamp01(0.35 + state.Emotions.Fear*0.40 + state.Relationship.Tension*0.25 - state.Emotions.Trust*0.15),
		AssociationBiases: associationBiases(state),
		GeneratedAt:       now,
	}
	report.Text = compactText(report)
	return report
}

func toneBias(state State) string {
	switch {
	case state.Relationship.Tension > 0.65 || state.Emotions.Anger > 0.60:
		return "firm_contained"
	case state.Emotions.Sadness > 0.55 || state.Emotions.Fear > 0.55:
		return "gentle_supportive"
	case state.Emotions.Trust > 0.55 && state.Emotions.Joy > 0.40:
		return "warm_attentive"
	case state.Emotions.Anticipation > 0.55:
		return "engaged_forward"
	default:
		return "calm_neutral"
	}
}

func associationBiases(state State) []string {
	var biases []string
	if state.Emotions.Anger > 0.45 || state.Relationship.Tension > 0.50 {
		biases = append(biases, "boundary_protection")
	}
	if state.Emotions.Trust > 0.50 {
		biases = append(biases, "cooperation")
	}
	if state.Emotions.Joy > 0.50 {
		biases = append(biases, "exploration")
	}
	if state.Emotions.Anticipation > 0.40 {
		biases = append(biases, "future_planning")
	}
	if state.Emotions.Fear > 0.45 {
		biases = append(biases, "risk_scan")
	}
	if state.Emotions.Sadness > 0.45 {
		biases = append(biases, "care_and_recovery")
	}
	sort.Strings(biases)
	return biases
}

func compactText(report Report) string {
	dominant := make([]string, 0, len(report.DominantEmotions))
	for _, signal := range report.DominantEmotions {
		dominant = append(dominant, fmt.Sprintf("%s=%.2f", signal.Name, signal.Value))
	}
	return fmt.Sprintf(
		"status=%s; version=%d; dominant=%s; tone=%s; risk=%.2f; attachment=%.2f; tension=%.2f; safety=%.2f",
		report.Status,
		report.StateVersion,
		strings.Join(dominant, ","),
		report.ToneBias,
		report.RiskSensitivity,
		report.Relationship.Attachment,
		report.Relationship.Tension,
		report.Relationship.PerceivedSafety,
	)
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampSigned(value, limit float64) float64 {
	if value < -limit {
		return -limit
	}
	if value > limit {
		return limit
	}
	return value
}
