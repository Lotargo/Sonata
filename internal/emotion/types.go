package emotion

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Emotion string

const (
	Joy          Emotion = "joy"
	Trust        Emotion = "trust"
	Fear         Emotion = "fear"
	Surprise     Emotion = "surprise"
	Sadness      Emotion = "sadness"
	Disgust      Emotion = "disgust"
	Anger        Emotion = "anger"
	Anticipation Emotion = "anticipation"
)

var allEmotions = [...]Emotion{
	Joy,
	Trust,
	Fear,
	Surprise,
	Sadness,
	Disgust,
	Anger,
	Anticipation,
}

func AllEmotions() []Emotion {
	return append([]Emotion(nil), allEmotions[:]...)
}

func (emotion Emotion) Valid() bool {
	switch emotion {
	case Joy, Trust, Fear, Surprise, Sadness, Disgust, Anger, Anticipation:
		return true
	default:
		return false
	}
}

type Vector struct {
	Joy          float64
	Trust        float64
	Fear         float64
	Surprise     float64
	Sadness      float64
	Disgust      float64
	Anger        float64
	Anticipation float64
}

func VectorFromMap(values map[string]float64) (Vector, error) {
	if len(values) != len(allEmotions) {
		return Vector{}, fmt.Errorf("emotion vector requires exactly %d values", len(allEmotions))
	}
	var vector Vector
	for _, emotion := range allEmotions {
		value, exists := values[string(emotion)]
		if !exists {
			return Vector{}, fmt.Errorf("emotion vector is missing %q", emotion)
		}
		if value < 0 || value > 1 {
			return Vector{}, fmt.Errorf("emotion %s must be between 0 and 1", emotion)
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

func (vector Vector) Get(emotion Emotion) float64 {
	switch emotion {
	case Joy:
		return vector.Joy
	case Trust:
		return vector.Trust
	case Fear:
		return vector.Fear
	case Surprise:
		return vector.Surprise
	case Sadness:
		return vector.Sadness
	case Disgust:
		return vector.Disgust
	case Anger:
		return vector.Anger
	case Anticipation:
		return vector.Anticipation
	default:
		return 0
	}
}

func (vector *Vector) Set(emotion Emotion, value float64) {
	switch emotion {
	case Joy:
		vector.Joy = value
	case Trust:
		vector.Trust = value
	case Fear:
		vector.Fear = value
	case Surprise:
		vector.Surprise = value
	case Sadness:
		vector.Sadness = value
	case Disgust:
		vector.Disgust = value
	case Anger:
		vector.Anger = value
	case Anticipation:
		vector.Anticipation = value
	}
}

func (vector Vector) Map() map[string]float64 {
	result := make(map[string]float64, len(allEmotions))
	for _, emotion := range allEmotions {
		result[string(emotion)] = vector.Get(emotion)
	}
	return result
}

func (vector Vector) Validate() error {
	for _, emotion := range allEmotions {
		value := vector.Get(emotion)
		if value < 0 || value > 1 {
			return fmt.Errorf("emotion %s must be between 0 and 1", emotion)
		}
	}
	return nil
}

type RelationshipState struct {
	Attachment       float64
	Openness         float64
	Tension          float64
	ConfidenceInUser float64
	PerceivedSafety  float64
	UnresolvedHurt   float64
}

func (relationship RelationshipState) Validate() error {
	values := map[string]float64{
		"attachment":         relationship.Attachment,
		"openness":           relationship.Openness,
		"tension":            relationship.Tension,
		"confidence_in_user": relationship.ConfidenceInUser,
		"perceived_safety":   relationship.PerceivedSafety,
		"unresolved_hurt":    relationship.UnresolvedHurt,
	}
	for name, value := range values {
		if value < 0 || value > 1 {
			return fmt.Errorf("relationship %s must be between 0 and 1", name)
		}
	}
	return nil
}

type StateKey struct {
	IdentityID string
	UserID     string
}

func (key StateKey) Validate() error {
	if strings.TrimSpace(key.IdentityID) == "" {
		return errors.New("emotion identity ID is required")
	}
	if strings.TrimSpace(key.UserID) == "" {
		return errors.New("emotion user ID is required")
	}
	return nil
}

type State struct {
	Key           StateKey
	Emotions      Vector
	Relationship  RelationshipState
	Fatigue       float64
	Stability     float64
	LastUpdatedAt time.Time
	Version       int64
}

func (state State) Validate() error {
	if err := state.Key.Validate(); err != nil {
		return err
	}
	if err := state.Emotions.Validate(); err != nil {
		return err
	}
	if err := state.Relationship.Validate(); err != nil {
		return err
	}
	if state.Fatigue < 0 || state.Fatigue > 1 {
		return errors.New("fatigue must be between 0 and 1")
	}
	if state.Stability < 0 || state.Stability > 1 {
		return errors.New("stability must be between 0 and 1")
	}
	if state.Version < 0 {
		return errors.New("state version cannot be negative")
	}
	if state.LastUpdatedAt.IsZero() {
		return errors.New("last updated time is required")
	}
	return nil
}

type StimulusKind string

const (
	StimulusUserWarmth          StimulusKind = "user_warmth"
	StimulusUserHostility       StimulusKind = "user_hostility"
	StimulusUserTrust           StimulusKind = "user_trust"
	StimulusUserRejection       StimulusKind = "user_rejection"
	StimulusUserDistress        StimulusKind = "user_distress"
	StimulusUserSuccess         StimulusKind = "user_success"
	StimulusUserApology         StimulusKind = "user_apology"
	StimulusUserBoundary        StimulusKind = "user_boundary"
	StimulusConversationReturn  StimulusKind = "conversation_return"
	StimulusConversationBreak   StimulusKind = "conversation_break"
	StimulusPromiseKept         StimulusKind = "promise_kept"
	StimulusPromiseBroken       StimulusKind = "promise_broken"
	StimulusToolSuccess         StimulusKind = "tool_success"
	StimulusToolFailure         StimulusKind = "tool_failure"
	StimulusResponseRejected    StimulusKind = "response_rejected"
	StimulusResponseAppreciated StimulusKind = "response_appreciated"
)

func (kind StimulusKind) Valid() bool {
	switch kind {
	case StimulusUserWarmth,
		StimulusUserHostility,
		StimulusUserTrust,
		StimulusUserRejection,
		StimulusUserDistress,
		StimulusUserSuccess,
		StimulusUserApology,
		StimulusUserBoundary,
		StimulusConversationReturn,
		StimulusConversationBreak,
		StimulusPromiseKept,
		StimulusPromiseBroken,
		StimulusToolSuccess,
		StimulusToolFailure,
		StimulusResponseRejected,
		StimulusResponseAppreciated:
		return true
	default:
		return false
	}
}

type Stimulus struct {
	Kind       StimulusKind
	Source     string
	Intensity  float64
	Confidence float64
	Valence    float64
	Arousal    float64
	Target     string
	CreatedAt  time.Time
	Metadata   map[string]string
}

func (stimulus Stimulus) Validate() error {
	if !stimulus.Kind.Valid() {
		return fmt.Errorf("unsupported stimulus kind %q", stimulus.Kind)
	}
	if strings.TrimSpace(stimulus.Source) == "" {
		return errors.New("stimulus source is required")
	}
	if stimulus.Intensity < 0 || stimulus.Intensity > 1 {
		return errors.New("stimulus intensity must be between 0 and 1")
	}
	if stimulus.Confidence < 0 || stimulus.Confidence > 1 {
		return errors.New("stimulus confidence must be between 0 and 1")
	}
	if stimulus.Valence < -1 || stimulus.Valence > 1 {
		return errors.New("stimulus valence must be between -1 and 1")
	}
	if stimulus.Arousal < -1 || stimulus.Arousal > 1 {
		return errors.New("stimulus arousal must be between -1 and 1")
	}
	for key := range stimulus.Metadata {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") || strings.Contains(normalized, "password") || strings.Contains(normalized, "api_key") {
			return fmt.Errorf("stimulus metadata key %q is not allowed", key)
		}
	}
	return nil
}

type EmotionSignal struct {
	Name  Emotion
	Value float64
}

type RelationshipReport struct {
	Attachment       float64
	Openness         float64
	Tension          float64
	ConfidenceInUser float64
	PerceivedSafety  float64
	UnresolvedHurt   float64
}

type Status string

const (
	StatusHealthy  Status = "HEALTHY"
	StatusDegraded Status = "DEGRADED"
)

type Report struct {
	Text              string
	Status            Status
	StateVersion      int64
	DominantEmotions  []EmotionSignal
	Relationship      RelationshipReport
	Fatigue           float64
	Stability         float64
	ToneBias          string
	RiskSensitivity   float64
	AssociationBiases []string
	GeneratedAt       time.Time
}

func (report Report) Validate() error {
	if report.Status != StatusHealthy && report.Status != StatusDegraded {
		return fmt.Errorf("invalid emotion report status %q", report.Status)
	}
	if report.StateVersion < 0 {
		return errors.New("emotion report version cannot be negative")
	}
	if report.GeneratedAt.IsZero() {
		return errors.New("emotion report generation time is required")
	}
	if report.Fatigue < 0 || report.Fatigue > 1 || report.Stability < 0 || report.Stability > 1 || report.RiskSensitivity < 0 || report.RiskSensitivity > 1 {
		return errors.New("emotion report scalar values must be between 0 and 1")
	}
	for _, signal := range report.DominantEmotions {
		if !signal.Name.Valid() || signal.Value < 0 || signal.Value > 1 {
			return errors.New("emotion report contains invalid dominant emotion")
		}
	}
	if !sort.StringsAreSorted(append([]string(nil), report.AssociationBiases...)) {
		return errors.New("emotion report association biases must be sorted")
	}
	return nil
}
