package conservation

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0007(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar-task-0007", domain.RoleRegistrar)
	conservator := conservationUser(t, store, "conservator-task-0007", domain.RoleConservator)
	service := conservationService(store)
	registered, err := service.Register(conservationContext(registrar), RegisterArtifactInput{
		AccessionNo: "HG-TASK-0007", Name: "Stone relief", Material: "stone", Period: "Han",
		RiskClass: domain.RiskLow, InitialZoneID: "zone-general", Summary: "stable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0007 AFTER INSERT ON quarantine_cases BEGIN SELECT RAISE(ABORT, 'case failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenQuarantine(conservationContext(conservator), QuarantineInput{ArtifactID: registered.Artifact.ID, ZoneID: "zone-general", Reason: "temporary review"}); err == nil {
		t.Fatal("quarantine should report the case persistence failure")
	}
	loaded, err := store.GetArtifact(context.Background(), registered.Artifact.TenantID, registered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	var occupied int
	if err := store.DB.QueryRow(`SELECT occupied FROM quarantine_zones WHERE id = 'zone-general'`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.ArtifactRegistered || occupied != 0 {
		t.Fatalf("failed quarantine left ownership split: artifact=%s occupied=%d", loaded.Status, occupied)
	}
}
