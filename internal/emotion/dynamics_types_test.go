package emotion

import (
	"math"
	"testing"
	"time"
)

func TestBoundedGenericRejectsInvalidValuesAndClampsUpdates(t *testing.T) {
	t.Parallel()

	bounds := Bounds[float64]{Min: -1, Max: 1}
	value, err := NewBounded(0.25, bounds)
	if err != nil {
		t.Fatalf("create bounded value: %v", err)
	}
	updated, err := value.Add(2)
	if err != nil {
		t.Fatalf("add bounded delta: %v", err)
	}
	if updated.Value() != 1 {
		t.Fatalf("expected clamped value 1, got %v", updated.Value())
	}
	if _, err := NewBounded(math.NaN(), bounds); err == nil {
		t.Fatal("expected NaN to be rejected")
	}
	if _, err := NewBounded(0.5, Bounds[float64]{Min: 2, Max: 1}); err == nil {
		t.Fatal("expected inverted bounds to be rejected")
	}
}

func TestDomainScalarsRejectOutOfRangeAndNonFiniteValues(t *testing.T) {
	t.Parallel()

	invalidUnits := []float64{-0.01, 1.01, math.NaN(), math.Inf(1)}
	for _, value := range invalidUnits {
		if _, err := NewUnit(value); err == nil {
			t.Fatalf("expected unit value %v to be rejected", value)
		}
	}
	invalidSigned := []float64{-1.01, 1.01, math.NaN(), math.Inf(-1)}
	for _, value := range invalidSigned {
		if _, err := NewSignedUnit(value); err == nil {
			t.Fatalf("expected signed unit value %v to be rejected", value)
		}
	}
	invalidNonNegative := []float64{-0.01, math.NaN(), math.Inf(1)}
	for _, value := range invalidNonNegative {
		if _, err := NewNonNegative(value); err == nil {
			t.Fatalf("expected non-negative value %v to be rejected", value)
		}
	}
}

func TestBaselinePersonalityPhysiologyAndDrivesValidate(t *testing.T) {
	t.Parallel()

	if err := NeutralPersonality().Validate(); err != nil {
		t.Fatalf("validate neutral personality: %v", err)
	}
	if err := BaselinePhysiology().Validate(); err != nil {
		t.Fatalf("validate baseline physiology: %v", err)
	}
	if err := ValidateDrives(BaselineDrives()); err != nil {
		t.Fatalf("validate baseline drives: %v", err)
	}
}

func TestDriveValidationRejectsDuplicateAndUnstableOrder(t *testing.T) {
	t.Parallel()

	duplicate := []DriveState{
		{Kind: DriveCognition, Level: 1, Satisfaction: 1, Urgency: 0},
		{Kind: DriveCognition, Level: 1, Satisfaction: 1, Urgency: 0},
	}
	if err := ValidateDrives(duplicate); err == nil {
		t.Fatal("expected duplicate drive to be rejected")
	}

	unstable := []DriveState{
		{Kind: DriveSocialConnection, Level: 1, Satisfaction: 1, Urgency: 0},
		{Kind: DriveCognition, Level: 1, Satisfaction: 1, Urgency: 0},
	}
	if err := ValidateDrives(unstable); err == nil {
		t.Fatal("expected unstable drive order to be rejected")
	}
}

func TestComplexStateAndEvidenceValidation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	states := []ComplexState{
		{Kind: ComplexStateChronicStress, DefinitionID: "state-chronic-stress-v1", Activation: Unit(0.4), ActiveSince: now},
		{Kind: ComplexStateDepressive, DefinitionID: "state-depressive-v1", Activation: Unit(0.7), ActiveSince: now},
	}
	if err := ValidateComplexStates(states); err != nil {
		t.Fatalf("validate complex states: %v", err)
	}

	evidence := []StateEvidence{
		{
			Kind:         ComplexStateChronicStress,
			DefinitionID: "state-chronic-stress-v1",
			Evidence: EvidenceAccumulator{
				PositiveArea:  NonNegative(4.5),
				ViolationArea: NonNegative(0.2),
				ObservedFor:   12 * time.Hour,
				LastUpdatedAt: now,
			},
		},
		{
			Kind:         ComplexStateDepressive,
			DefinitionID: "state-depressive-v1",
			Evidence: EvidenceAccumulator{
				PositiveArea:  NonNegative(8.5),
				ViolationArea: 0,
				ObservedFor:   48 * time.Hour,
				LastUpdatedAt: now,
			},
		},
	}
	if err := ValidateStateEvidence(evidence); err != nil {
		t.Fatalf("validate state evidence: %v", err)
	}
}

func TestActiveComplexStateRequiresVersionAndTimestamp(t *testing.T) {
	t.Parallel()

	state := ComplexState{Kind: ComplexStateDepressive, Activation: Unit(0.5)}
	if err := state.Validate(); err == nil {
		t.Fatal("expected active complex state without definition ID and timestamp to be rejected")
	}
}

func TestObservedEvidenceRequiresTimestamp(t *testing.T) {
	t.Parallel()

	evidence := EvidenceAccumulator{ObservedFor: time.Hour}
	if err := evidence.Validate(); err == nil {
		t.Fatal("expected observed evidence without timestamp to be rejected")
	}
}
