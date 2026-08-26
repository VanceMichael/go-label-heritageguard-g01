package loan

import (
	"errors"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0021(t *testing.T) {
	if _, _, _, err := custodyTransition(domain.LoanDispatched, "returned", "loan-task-0021"); !errors.Is(err, domain.ErrIllegalState) {
		t.Fatalf("a dispatched shipment must pass through returning before receipt, got %v", err)
	}
	status, artifactStatus, activeLoan, err := custodyTransition(domain.LoanReturning, "returned", "loan-task-0021")
	if err != nil || status != domain.LoanReturned || artifactStatus != domain.ArtifactAssessment || activeLoan != "" {
		t.Fatalf("valid return transition changed its ownership contract: %s %s %q %v", status, artifactStatus, activeLoan, err)
	}
}
