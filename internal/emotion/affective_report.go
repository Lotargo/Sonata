package emotion

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// AffectiveReport is the safe application projection of the canonical v1
// state. It deliberately excludes raw messages, event history, definition IDs,
// evidence accumulators and internal transition coefficients.
type AffectiveReport struct {
	Text           string
	Status         Status
	ProfileVersion string
	StateVersion   int64
	GeneratedAt    time.Time
}

func (report AffectiveReport) Validate() error {
	if report.Status != StatusHealthy && report.Status != StatusDegraded {
		return fmt.Errorf("invalid affective report status %q", report.Status)
	}
	if strings.TrimSpace(report.ProfileVersion) == "" {
		return errors.New("affective report profile version is required")
	}
	if report.StateVersion < 0 {
		return errors.New("affective report state version cannot be negative")
	}
	if report.GeneratedAt.IsZero() {
		return errors.New("affective report generation time is required")
	}
	if strings.TrimSpace(report.Text) == "" {
		return errors.New("affective report text is required")
	}
	return nil
}

func buildAffectiveReport(state AffectiveState, status Status, generatedAt time.Time) (AffectiveReport, error) {
	if err := state.Validate(); err != nil {
		return AffectiveReport{}, fmt.Errorf("validate affective report state: %w", err)
	}
	generatedAt = generatedAt.UTC()
	if generatedAt.IsZero() {
		return AffectiveReport{}, errors.New("affective report generation time is required")
	}

	dominant := make([]EmotionSignal, 0, len(allEmotions))
	for _, emotion := range allEmotions {
		dominant = append(dominant, EmotionSignal{Name: emotion, Value: state.Emotions.Get(emotion)})
	}
	sort.SliceStable(dominant, func(left, right int) bool {
		if dominant[left].Value == dominant[right].Value {
			return dominant[left].Name < dominant[right].Name
		}
		return dominant[left].Value > dominant[right].Value
	})
	if len(dominant) > 3 {
		dominant = dominant[:3]
	}
	dominantText := make([]string, 0, len(dominant))
	for _, signal := range dominant {
		dominantText = append(dominantText, fmt.Sprintf("%s=%.2f", signal.Name, signal.Value))
	}

	activeStates := make([]string, 0, len(state.ComplexStates))
	for _, active := range state.ComplexStates {
		if active.Active() {
			activeStates = append(activeStates, fmt.Sprintf("%s=%.2f", active.Kind, active.Activation.Float64()))
		}
	}
	sort.Strings(activeStates)
	if len(activeStates) == 0 {
		activeStates = append(activeStates, "none")
	}

	drives := append([]DriveState(nil), state.Drives...)
	sort.SliceStable(drives, func(left, right int) bool {
		if drives[left].Urgency == drives[right].Urgency {
			return drives[left].Kind < drives[right].Kind
		}
		return drives[left].Urgency > drives[right].Urgency
	})
	if len(drives) > 2 {
		drives = drives[:2]
	}
	driveText := make([]string, 0, len(drives))
	for _, drive := range drives {
		driveText = append(driveText, fmt.Sprintf("%s=%.2f", drive.Kind, drive.Urgency.Float64()))
	}

	report := AffectiveReport{
		Status:         status,
		ProfileVersion: state.ProfileVersion,
		StateVersion:   state.Version,
		GeneratedAt:    generatedAt,
	}
	report.Text = fmt.Sprintf(
		"status=%s; profile=%s; version=%d; dominant=%s; tone=%s; risk=%.2f; physiology=fatigue=%.2f,arousal=%.2f,energy=%.2f,stress=%.2f,stability=%.2f; relationship=attachment=%.2f,openness=%.2f,tension=%.2f,confidence=%.2f,safety=%.2f,hurt=%.2f; active_states=%s; drive_urgency=%s",
		report.Status,
		report.ProfileVersion,
		report.StateVersion,
		strings.Join(dominantText, ","),
		affectiveToneBias(state),
		affectiveRiskSensitivity(state),
		state.Physiology.Fatigue.Float64(),
		state.Physiology.Arousal.Float64(),
		state.Physiology.Energy.Float64(),
		state.Physiology.StressLoad.Float64(),
		state.Physiology.Stability.Float64(),
		state.Relationship.Attachment,
		state.Relationship.Openness,
		state.Relationship.Tension,
		state.Relationship.ConfidenceInUser,
		state.Relationship.PerceivedSafety,
		state.Relationship.UnresolvedHurt,
		strings.Join(activeStates, ","),
		strings.Join(driveText, ","),
	)
	if err := report.Validate(); err != nil {
		return AffectiveReport{}, err
	}
	return report, nil
}

func affectiveToneBias(state AffectiveState) string {
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

func affectiveRiskSensitivity(state AffectiveState) float64 {
	return clampFloat(
		0.30+
			state.Emotions.Fear*0.35+
			state.Relationship.Tension*0.25+
			state.Physiology.StressLoad.Float64()*0.20-
			state.Emotions.Trust*0.10-
			state.Relationship.PerceivedSafety*0.10,
		0,
		1,
	)
}
