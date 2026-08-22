package environment

import (
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type Policy struct {
	MinimumSamples int
	MaximumGap     time.Duration
	AllowedDrift   float64
}

func DefaultPolicy() Policy {
	return Policy{MinimumSamples: 3, MaximumGap: 20 * time.Minute, AllowedDrift: 0.5}
}

func (p Policy) Validate() error {
	if p.MinimumSamples < 1 {
		return domain.FieldError{Field: "minimum_samples", Message: "must be positive"}
	}
	if p.MaximumGap <= 0 {
		return domain.FieldError{Field: "maximum_gap", Message: "must be positive"}
	}
	if p.AllowedDrift < 0 {
		return domain.FieldError{Field: "allowed_drift", Message: "cannot be negative"}
	}
	return nil
}

func (p Policy) CheckAssessment(assessment domain.EnvironmentAssessment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if assessment.ReadingCount < p.MinimumSamples {
		return fmt.Errorf("only %d samples available: %w", assessment.ReadingCount, domain.ErrPrecondition)
	}
	if !assessment.Ready {
		return fmt.Errorf("environment has unresolved deviations: %w", domain.ErrPrecondition)
	}
	return nil
}
