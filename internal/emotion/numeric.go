package emotion

import (
	"errors"
	"fmt"
	"math"
)

// Float is the numeric constraint used by generic bounded helpers.
type Float interface {
	~float32 | ~float64
}

// Bounds describes an inclusive numeric interval.
type Bounds[T Float] struct {
	Min T
	Max T
}

func (bounds Bounds[T]) Validate() error {
	if !finite(float64(bounds.Min)) || !finite(float64(bounds.Max)) {
		return errors.New("bounds must be finite")
	}
	if bounds.Min > bounds.Max {
		return errors.New("bounds minimum cannot exceed maximum")
	}
	return nil
}

// Bounded stores a value together with its validated inclusive bounds.
// The fields are intentionally private so callers cannot bypass validation.
type Bounded[T Float] struct {
	value  T
	bounds Bounds[T]
}

func NewBounded[T Float](value T, bounds Bounds[T]) (Bounded[T], error) {
	if err := bounds.Validate(); err != nil {
		return Bounded[T]{}, err
	}
	if !finite(float64(value)) {
		return Bounded[T]{}, errors.New("bounded value must be finite")
	}
	if value < bounds.Min || value > bounds.Max {
		return Bounded[T]{}, fmt.Errorf("value %v must be between %v and %v", value, bounds.Min, bounds.Max)
	}
	return Bounded[T]{value: value, bounds: bounds}, nil
}

func NewClampedBounded[T Float](value T, bounds Bounds[T]) (Bounded[T], error) {
	if err := bounds.Validate(); err != nil {
		return Bounded[T]{}, err
	}
	if !finite(float64(value)) {
		return Bounded[T]{}, errors.New("bounded value must be finite")
	}
	return Bounded[T]{value: clamp(value, bounds.Min, bounds.Max), bounds: bounds}, nil
}

func (value Bounded[T]) Value() T {
	return value.value
}

func (value Bounded[T]) Bounds() Bounds[T] {
	return value.bounds
}

func (value Bounded[T]) Add(delta T) (Bounded[T], error) {
	if !finite(float64(delta)) {
		return Bounded[T]{}, errors.New("bounded delta must be finite")
	}
	return NewClampedBounded(value.value+delta, value.bounds)
}

func clamp[T Float](value, minimum, maximum T) T {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// Unit is a domain scalar in the inclusive interval [0, 1].
type Unit float64

func NewUnit(value float64) (Unit, error) {
	if !finite(value) {
		return 0, errors.New("unit value must be finite")
	}
	if value < 0 || value > 1 {
		return 0, fmt.Errorf("unit value %.6f must be between 0 and 1", value)
	}
	return Unit(value), nil
}

func NewClampedUnit(value float64) (Unit, error) {
	if !finite(value) {
		return 0, errors.New("unit value must be finite")
	}
	return Unit(clamp(value, 0.0, 1.0)), nil
}

func (value Unit) Validate() error {
	_, err := NewUnit(float64(value))
	return err
}

func (value Unit) Float64() float64 {
	return float64(value)
}

// SignedUnit is a domain scalar in the inclusive interval [-1, 1].
type SignedUnit float64

func NewSignedUnit(value float64) (SignedUnit, error) {
	if !finite(value) {
		return 0, errors.New("signed unit value must be finite")
	}
	if value < -1 || value > 1 {
		return 0, fmt.Errorf("signed unit value %.6f must be between -1 and 1", value)
	}
	return SignedUnit(value), nil
}

func NewClampedSignedUnit(value float64) (SignedUnit, error) {
	if !finite(value) {
		return 0, errors.New("signed unit value must be finite")
	}
	return SignedUnit(clamp(value, -1.0, 1.0)), nil
}

func (value SignedUnit) Validate() error {
	_, err := NewSignedUnit(float64(value))
	return err
}

func (value SignedUnit) Float64() float64 {
	return float64(value)
}

// Multiplier is a bounded response coefficient in [0, 2].
// Values below 1 suppress a channel, 1 preserves it and values above 1 amplify it.
type Multiplier float64

func NewMultiplier(value float64) (Multiplier, error) {
	if !finite(value) {
		return 0, errors.New("multiplier must be finite")
	}
	if value < 0 || value > 2 {
		return 0, fmt.Errorf("multiplier %.6f must be between 0 and 2", value)
	}
	return Multiplier(value), nil
}

func (value Multiplier) Validate() error {
	_, err := NewMultiplier(float64(value))
	return err
}

func (value Multiplier) Float64() float64 {
	return float64(value)
}

// NonNegative is a finite domain scalar in [0, +inf).
type NonNegative float64

func NewNonNegative(value float64) (NonNegative, error) {
	if !finite(value) {
		return 0, errors.New("non-negative value must be finite")
	}
	if value < 0 {
		return 0, fmt.Errorf("non-negative value %.6f cannot be negative", value)
	}
	return NonNegative(value), nil
}

func (value NonNegative) Validate() error {
	_, err := NewNonNegative(float64(value))
	return err
}

func (value NonNegative) Float64() float64 {
	return float64(value)
}
