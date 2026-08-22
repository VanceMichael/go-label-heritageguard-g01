package conservation

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
)

type conservationLogWriter struct{}

func (conservationLogWriter) Write(p []byte) (int, error) { return len(p), nil }

type conservationIDs struct{ index int }

func (g *conservationIDs) New(prefix string) string {
	g.index++
	return prefix + "-" + time.Duration(g.index).String()
}

func conservationStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(context.Background(), ":memory:", 1, slog.New(slog.NewTextHandler(conservationLogWriter{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func conservationUser(t *testing.T, store *sqlite.Store, id string, role domain.Role) domain.User {
	t.Helper()
	user := domain.User{ID: id, TenantID: "museum-demo", Email: id + "@museum.invalid", DisplayName: id,
		Role: role, PasswordHash: []byte("hash"), Active: true, Version: 1,
		CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

func conservationService(store *sqlite.Store) *Service {
	return &Service{Artifacts: store, Cases: store, IDs: &conservationIDs{}, Now: store.Now}
}

func conservationContext(user domain.User) context.Context {
	return service.WithPrincipal(service.WithRequestID(context.Background(), "conservation-request"), domain.Principal{
		TenantID: user.TenantID, UserID: user.ID, Role: user.Role,
	})
}

func TestRegisterRecordAndReleaseLowRiskArtifact(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar", domain.RoleRegistrar)
	conservator := conservationUser(t, store, "conservator", domain.RoleConservator)
	conservation := conservationService(store)
	registered, err := conservation.Register(conservationContext(registrar), RegisterArtifactInput{
		AccessionNo: "HG-001", Name: "Painted brick", Material: "clay", Period: "Western Xia",
		RiskClass: domain.RiskLow, InitialZoneID: "zone-general", Summary: "stable intake",
		Measurements: map[string]float64{"humidity": 48}, Issues: []string{"surface dust"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Artifact.Status != domain.ArtifactRegistered || registered.Report.ID == "" || registered.Custody.ID == "" {
		t.Fatalf("registration did not return complete intake record: %#v", registered)
	}
	audit, err := store.List(context.Background(), registrar.TenantID, registered.Artifact.ID, domain.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 || audit[0].Action != "artifact.register" || audit[0].RequestID != "conservation-request" {
		t.Fatalf("registration audit was not committed with intake transaction: %#v", audit)
	}
	report, err := conservation.RecordCondition(conservationContext(conservator), ConditionInput{
		ArtifactID: registered.Artifact.ID, Summary: "stable after inspection", Severity: domain.RiskLow,
		Measurements: map[string]float64{"humidity": 49}, Issues: nil, Final: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.GetArtifact(context.Background(), registrar.TenantID, registered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != domain.ArtifactAssessment || artifact.LastReportID != report.ID {
		t.Fatalf("condition did not advance assessment: %#v", artifact)
	}
	released, err := conservation.ReleaseAssessment(conservationContext(conservator), artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != domain.ArtifactReady || released.Version != artifact.Version+1 {
		t.Fatalf("assessment release wrong: %#v", released)
	}
}

func TestHighRiskAssessmentRequiresQuarantine(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar", domain.RoleRegistrar)
	conservator := conservationUser(t, store, "conservator", domain.RoleConservator)
	service := conservationService(store)
	registered, err := service.Register(conservationContext(registrar), RegisterArtifactInput{
		AccessionNo: "HG-002", Name: "Silk banner", Material: "silk", Period: "Ming",
		RiskClass: domain.RiskHigh, InitialZoneID: "zone-general", Summary: "fragile intake",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCondition(conservationContext(conservator), ConditionInput{
		ArtifactID: registered.Artifact.ID, Summary: "active pigment loss", Severity: domain.RiskHigh, Final: true,
	}); err != nil {
		t.Fatal(err)
	}
	artifact, err := store.GetArtifact(context.Background(), registrar.TenantID, registered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReleaseAssessment(conservationContext(conservator), artifact.ID); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("expected high-risk release to be blocked, got %v", err)
	}
	quarantine, err := service.OpenQuarantine(conservationContext(conservator), QuarantineInput{
		ArtifactID: artifact.ID, ZoneID: "zone-general", Reason: "pigment loss requires isolation",
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetArtifact(context.Background(), artifact.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.ArtifactQuarantined || loaded.CurrentCaseID != quarantine.ID {
		t.Fatalf("artifact was not moved into quarantine: %#v", loaded)
	}
}

func TestTreatmentApprovalExecutionAndCompletionRestoresArtifact(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar", domain.RoleRegistrar)
	conservator := conservationUser(t, store, "conservator", domain.RoleConservator)
	supervisor := conservationUser(t, store, "supervisor", domain.RoleSupervisor)
	conservation := conservationService(store)
	registered, err := conservation.Register(conservationContext(registrar), RegisterArtifactInput{
		AccessionNo: "HG-003", Name: "Paper map", Material: "paper", Period: "Qing",
		RiskClass: domain.RiskCritical, InitialZoneID: "zone-paper", Summary: "water damage",
	})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := conservation.OpenQuarantine(conservationContext(conservator), QuarantineInput{
		ArtifactID: registered.Artifact.ID, ZoneID: "zone-paper", Reason: "water damage",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := conservation.DraftTreatment(conservationContext(conservator), TreatmentInput{
		ArtifactID: registered.Artifact.ID, QuarantineID: quarantine.ID, Procedure: "deacidification and drying",
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := conservation.AdvanceTreatment(conservationContext(supervisor), plan.ID, domain.TreatmentApproved, "")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.TreatmentApproved || approved.ApprovedBy != supervisor.ID {
		t.Fatalf("approval state wrong: %#v", approved)
	}
	inProgress, err := conservation.AdvanceTreatment(conservationContext(conservator), plan.ID, domain.TreatmentInProgress, "")
	if err != nil {
		t.Fatal(err)
	}
	if inProgress.Status != domain.TreatmentInProgress {
		t.Fatalf("in-progress state wrong: %#v", inProgress)
	}
	if _, err := conservation.AdvanceTreatment(conservationContext(conservator), plan.ID, domain.TreatmentCompleted, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("completion without evidence should fail, got %v", err)
	}
	completed, err := conservation.AdvanceTreatment(conservationContext(conservator), plan.ID, domain.TreatmentCompleted, "s3://evidence/hg-003/report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domain.TreatmentCompleted || completed.CompletedAt == nil {
		t.Fatalf("completed treatment wrong: %#v", completed)
	}
	artifact, err := store.GetArtifact(context.Background(), registrar.TenantID, registered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != domain.ArtifactReady || artifact.CurrentCaseID != "" {
		t.Fatalf("completed treatment did not restore artifact: %#v", artifact)
	}
	var occupied int
	if err := store.DB.QueryRow(`SELECT occupied FROM quarantine_zones WHERE id = 'zone-paper'`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if occupied != 0 {
		t.Fatalf("quarantine capacity was not released: %d", occupied)
	}
}

func TestConservationRejectsWrongRolesAndCancelledContext(t *testing.T) {
	store := conservationStore(t)
	registrar := conservationUser(t, store, "registrar", domain.RoleRegistrar)
	service := conservationService(store)
	if _, err := service.RecordCondition(conservationContext(registrar), ConditionInput{ArtifactID: "missing", Summary: "x", Severity: domain.RiskLow}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("registrar should not inspect condition, got %v", err)
	}
	ctx, cancel := context.WithCancel(conservationContext(registrar))
	cancel()
	if _, err := service.Register(ctx, RegisterArtifactInput{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("register validation should reject empty input, got %v", err)
	}
}
