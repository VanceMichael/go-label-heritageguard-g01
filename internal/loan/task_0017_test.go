package loan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0017(t *testing.T) {
	store := loanStore(t)
	registrar := loanUser(t, store, "registrar-task-0017", domain.RoleRegistrar)
	artifact := loanArtifact(t, store, "artifact-task-0017")
	start, end := loanPeriod(store)
	if err := store.CreateLoan(context.Background(), domain.LoanRequest{ID: "returning-task-0017", TenantID: artifact.TenantID, ArtifactID: artifact.ID, Borrower: "Regional Museum", Purpose: "return inspection", StartAt: start, EndAt: end, Status: domain.LoanReturning, Version: 1, CreatedBy: registrar.ID, CreatedAt: store.Now(), UpdatedAt: store.Now()}); err != nil {
		t.Fatal(err)
	}
	service := loanService(store)
	if _, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Another Museum", Purpose: "overlapping exhibition", StartAt: start.Add(time.Hour), EndAt: end.Add(time.Hour)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("an in-return loan must reserve its time window, got %v", err)
	}
}
