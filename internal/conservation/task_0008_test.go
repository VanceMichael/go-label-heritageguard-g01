package conservation

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0008(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar-task-0008", domain.RoleRegistrar)
	conservator := conservationUser(t, store, "conservator-task-0008", domain.RoleConservator)
	service := conservationService(store)
	registered, err := service.Register(conservationContext(registrar), RegisterArtifactInput{AccessionNo: "HG-TASK-0008", Name: "Silk scroll", Material: "silk", Period: "Ming", RiskClass: domain.RiskHigh, InitialZoneID: "zone-general", Summary: "fragile"})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := service.OpenQuarantine(conservationContext(conservator), QuarantineInput{ArtifactID: registered.Artifact.ID, ZoneID: "zone-general", Reason: "fragile pigments"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.DraftTreatment(conservationContext(conservator), TreatmentInput{ArtifactID: registered.Artifact.ID, QuarantineID: quarantine.ID, Procedure: "surface consolidation"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AdvanceTreatment(conservationContext(conservator), plan.ID, domain.TreatmentApproved, ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("only a supervisor may approve a treatment plan, got %v", err)
	}
	loaded, err := store.GetTreatmentPlan(context.Background(), conservator.TenantID, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.TreatmentDraft || loaded.ApprovedBy != "" {
		t.Fatalf("unauthorized approval changed the workflow: %#v", loaded)
	}
}
