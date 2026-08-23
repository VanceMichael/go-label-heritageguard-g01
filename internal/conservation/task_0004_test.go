package conservation

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0004(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar-task-0004", domain.RoleRegistrar)
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0004 AFTER INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	service := conservationService(store)
	_, err := service.Register(conservationContext(registrar), RegisterArtifactInput{
		AccessionNo: "HG-TASK-0004", Name: "Bronze plaque", Material: "bronze", Period: "Tang",
		RiskClass: domain.RiskLow, InitialZoneID: "zone-general", Summary: "intake condition",
	})
	if err == nil {
		t.Fatal("registration should report the condition persistence failure")
	}
	var artifacts, reports, custody int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE accession_no = 'HG-TASK-0004'`).Scan(&artifacts); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM condition_reports`).Scan(&reports); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM custody_events WHERE kind = 'registered'`).Scan(&custody); err != nil {
		t.Fatal(err)
	}
	if artifacts != 0 || reports != 0 || custody != 0 {
		t.Fatalf("failed registration left a partial intake aggregate: artifacts=%d reports=%d custody=%d", artifacts, reports, custody)
	}
}
