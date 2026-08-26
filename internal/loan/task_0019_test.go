package loan

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0019(t *testing.T) {
	store := loanStore(t)
	registrar := loanUser(t, store, "registrar-task-0019", domain.RoleRegistrar)
	coordinator := loanUser(t, store, "coordinator-task-0019", domain.RoleCoordinator)
	supervisor := loanUser(t, store, "supervisor-task-0019", domain.RoleSupervisor)
	artifact := loanArtifact(t, store, "artifact-task-0019")
	service := loanService(store)
	start, end := loanPeriod(store)
	item, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Regional Museum", Purpose: "joint exhibition", StartAt: start, EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Submit(loanContext(registrar), item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Approve(loanContext(supervisor), item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0019 AFTER UPDATE ON artifacts BEGIN SELECT RAISE(ABORT, 'artifact custody failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "vault", ToHolder: "packing room", Location: "packing room", SealNumber: "SEAL-19", Kind: "packed"}); err == nil {
		t.Fatal("custody recording should report the artifact update failure")
	}
	var custody int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM custody_events WHERE loan_id = ?`, item.ID).Scan(&custody); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetLoan(context.Background(), item.TenantID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if custody != 0 || loaded.Status != domain.LoanApproved {
		t.Fatalf("failed custody left an incomplete handoff: custody=%d status=%s", custody, loaded.Status)
	}
}
