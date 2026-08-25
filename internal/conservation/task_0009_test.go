package conservation

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0009(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar-task-0009", domain.RoleRegistrar)
	conservator := conservationUser(t, store, "conservator-task-0009", domain.RoleConservator)
	supervisor := conservationUser(t, store, "supervisor-task-0009", domain.RoleSupervisor)
	service := conservationService(store)
	registered, err := service.Register(conservationContext(registrar), RegisterArtifactInput{AccessionNo: "HG-TASK-0009", Name: "Paper map", Material: "paper", Period: "Qing", RiskClass: domain.RiskCritical, InitialZoneID: "zone-paper", Summary: "water damage"})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := service.OpenQuarantine(conservationContext(conservator), QuarantineInput{ArtifactID: registered.Artifact.ID, ZoneID: "zone-paper", Reason: "water damage"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.DraftTreatment(conservationContext(conservator), TreatmentInput{ArtifactID: registered.Artifact.ID, QuarantineID: quarantine.ID, Procedure: "drying"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.AdvanceTreatment(conservationContext(supervisor), plan.ID, domain.TreatmentApproved, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = service.AdvanceTreatment(conservationContext(conservator), plan.ID, domain.TreatmentInProgress, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0009 AFTER UPDATE ON artifacts WHEN NEW.status = 'ready' BEGIN SELECT RAISE(ABORT, 'artifact release failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AdvanceTreatment(conservationContext(conservator), plan.ID, domain.TreatmentCompleted, "s3://evidence/task-0009.pdf"); err == nil {
		t.Fatal("treatment completion should report the artifact release failure")
	}
	loadedPlan, err := store.GetTreatmentPlan(context.Background(), registrar.TenantID, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedArtifact, err := store.GetArtifact(context.Background(), registrar.TenantID, registered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedPlan.Status == domain.TreatmentCompleted || loadedArtifact.Status == domain.ArtifactReady {
		t.Fatalf("failed completion left workflow split: plan=%s artifact=%s", loadedPlan.Status, loadedArtifact.Status)
	}
}
