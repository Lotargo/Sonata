package emotion

import (
	"math"
	"testing"
)

func TestClampedDomainConstructorsRejectNonFiniteValues(t *testing.T) {
	t.Parallel()

	if _, err := NewClampedUnit(math.NaN()); err == nil {
		t.Fatal("expected clamped unit to reject NaN")
	}
	if _, err := NewClampedSignedUnit(math.Inf(1)); err == nil {
		t.Fatal("expected clamped signed unit to reject infinity")
	}

	unit, err := NewClampedUnit(1.5)
	if err != nil {
		t.Fatal(err)
	}
	if unit != 1 {
		t.Fatalf("clamped unit = %v, want 1", unit)
	}

	signed, err := NewClampedSignedUnit(-2)
	if err != nil {
		t.Fatal(err)
	}
	if signed != -1 {
		t.Fatalf("clamped signed unit = %v, want -1", signed)
	}
}
