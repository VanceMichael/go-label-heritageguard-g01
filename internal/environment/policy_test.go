package environment

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestDefaultPolicyAcceptsReadyAssessment(t *testing.T) {
	policy := DefaultPolicy()
	assessment := domain.EnvironmentAssessment{ReadingCount: 3, Ready: true}
	if err := policy.CheckAssessment(assessment); err != nil {
		t.Fatalf("default policy rejected healthy assessment: %v", err)
	}
}

func TestPolicyRejectsInsufficientOrUnhealthyReadings(t *testing.T) {
	policy := DefaultPolicy()
	tests := []domain.EnvironmentAssessment{
		{ReadingCount: 2, Ready: true},
		{ReadingCount: 3, Ready: false},
	}
	for _, assessment := range tests {
		if err := policy.CheckAssessment(assessment); !errors.Is(err, domain.ErrPrecondition) {
			t.Errorf("expected precondition failure for %#v, got %v", assessment, err)
		}
	}
}

func TestPolicyValidation(t *testing.T) {
	tests := []Policy{
		{MinimumSamples: 0, MaximumGap: time.Minute, AllowedDrift: 0.1},
		{MinimumSamples: 1, MaximumGap: 0, AllowedDrift: 0.1},
		{MinimumSamples: 1, MaximumGap: time.Minute, AllowedDrift: -0.1},
	}
	for _, policy := range tests {
		if err := policy.Validate(); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("expected invalid policy, got %v", err)
		}
	}
}
