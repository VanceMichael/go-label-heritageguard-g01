package exhibition

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0016(t *testing.T) {
	store := exhibitionStore(t)
	coordinator := exhibitionUser(t, store, "coordinator-task-0016", domain.RoleCoordinator)
	service := exhibitionService(store)
	if _, err := store.DB.ExecContext(context.Background(), `UPDATE display_cases SET status = 'active' WHERE id = 'case-east-01'`); err != nil {
		t.Fatal(err)
	}
	incident, err := service.OpenIncident(context.Background(), coordinator.TenantID, IncidentInput{DisplayCaseID: "case-east-01", WindowKey: "task-0016-window", Kind: "humidity", Summary: "high humidity"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.AdvanceIncident(exhibitionContext(coordinator), incident.ID, domain.IncidentResponding, false); err != nil {
		t.Fatal(err)
	}
	if _, err = service.AdvanceIncident(exhibitionContext(coordinator), incident.ID, domain.IncidentMonitoring, false); err != nil {
		t.Fatal(err)
	}
	caseBefore, err := store.GetDisplayCase(context.Background(), coordinator.TenantID, "case-east-01")
	if err != nil || caseBefore.Status != domain.CaseIncident {
		t.Fatalf("monitoring did not reserve incident case: %#v %v", caseBefore, err)
	}
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0016 AFTER UPDATE ON display_cases BEGIN SELECT RAISE(ABORT, 'case recovery failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AdvanceIncident(exhibitionContext(coordinator), incident.ID, domain.IncidentClosed, true); err == nil {
		t.Fatal("incident close should report the case recovery failure")
	}
	loadedIncident, err := store.GetIncident(context.Background(), coordinator.TenantID, incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedCase, err := store.GetDisplayCase(context.Background(), coordinator.TenantID, "case-east-01")
	if err != nil {
		t.Fatal(err)
	}
	if loadedIncident.Status == domain.IncidentClosed || loadedCase.Status != domain.CaseIncident {
		t.Fatalf("failed close left incident and case split: incident=%s case=%s", loadedIncident.Status, loadedCase.Status)
	}
}
