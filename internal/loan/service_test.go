package loan

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

type loanLogWriter struct{}

func (loanLogWriter) Write(p []byte) (int, error) { return len(p), nil }

type loanIDs struct{ index int }

func (g *loanIDs) New(prefix string) string {
	g.index++
	return prefix + "-" + string(rune('a'+g.index))
}

func loanStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(context.Background(), ":memory:", 1, slog.New(slog.NewTextHandler(loanLogWriter{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func loanUser(t *testing.T, store *sqlite.Store, id string, role domain.Role) domain.User {
	t.Helper()
	user := domain.User{ID: id, TenantID: "museum-demo", Email: id + "@museum.invalid", DisplayName: id,
		Role: role, PasswordHash: []byte("hash"), Active: true, Version: 1, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

func loanArtifact(t *testing.T, store *sqlite.Store, id string) domain.Artifact {
	t.Helper()
	artifact := domain.Artifact{ID: id, TenantID: "museum-demo", AccessionNo: id, Name: "Ceramic vessel", Material: "ceramic",
		Period: "Western Xia", RiskClass: domain.RiskLow, Status: domain.ArtifactReady, CurrentZoneID: "zone-general", Version: 1,
		CreatedAt: store.Now(), UpdatedAt: store.Now()}
	_, err := store.DB.Exec(`INSERT INTO artifacts(id, tenant_id, accession_no, name, material, period, risk_class, status,
		current_zone_id, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, artifact.ID, artifact.TenantID,
		artifact.AccessionNo, artifact.Name, artifact.Material, artifact.Period, artifact.RiskClass, artifact.Status,
		artifact.CurrentZoneID, artifact.Version, artifact.CreatedAt.Format(time.RFC3339Nano), artifact.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func loanService(store *sqlite.Store) *Service {
	return &Service{Artifacts: store, Loans: store, Approver: store, IDs: &loanIDs{}, Now: store.Now}
}

func loanContext(user domain.User) context.Context {
	return service.WithPrincipal(service.WithRequestID(context.Background(), "loan-request"), domain.Principal{TenantID: user.TenantID, UserID: user.ID, Role: user.Role})
}

func loanPeriod(store *sqlite.Store) (time.Time, time.Time) {
	start := store.Now().Add(24 * time.Hour)
	return start, start.Add(48 * time.Hour)
}

func TestLoanApprovalAndCustodyLifecycle(t *testing.T) {
	store := loanStore(t)
	registrar := loanUser(t, store, "registrar", domain.RoleRegistrar)
	coordinator := loanUser(t, store, "coordinator", domain.RoleCoordinator)
	supervisor := loanUser(t, store, "supervisor", domain.RoleSupervisor)
	artifact := loanArtifact(t, store, "artifact-loan")
	service := loanService(store)
	start, end := loanPeriod(store)
	item, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Regional Museum", Purpose: "joint exhibition", StartAt: start, EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != domain.LoanDraft || item.Version != 1 {
		t.Fatalf("draft loan wrong: %#v", item)
	}
	submitted, err := service.Submit(loanContext(registrar), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != domain.LoanSubmitted || submitted.Version != 2 {
		t.Fatalf("submitted loan wrong: %#v", submitted)
	}
	approved, err := service.Approve(loanContext(supervisor), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.LoanApproved || approved.ApprovedBy != supervisor.ID {
		t.Fatalf("approved loan wrong: %#v", approved)
	}
	loadedArtifact, err := store.GetArtifact(context.Background(), artifact.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedArtifact.ActiveLoanID != item.ID || loadedArtifact.Status != domain.ArtifactReady {
		t.Fatalf("approval did not reserve active loan: %#v", loadedArtifact)
	}
	packed, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "vault", ToHolder: "packing room", Location: "packing room", SealNumber: "SEAL-1", Kind: "packed"})
	if err != nil {
		t.Fatal(err)
	}
	if packed.Kind != "packed" {
		t.Fatalf("packing custody wrong: %#v", packed)
	}
	if _, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "packing room", ToHolder: "courier", Location: "dispatch dock", SealNumber: "SEAL-1", Kind: "dispatched"}); err != nil {
		t.Fatal(err)
	}
	loadedArtifact, err = store.GetArtifact(context.Background(), artifact.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedArtifact.Status != domain.ArtifactOnLoan || loadedArtifact.ActiveLoanID != item.ID {
		t.Fatalf("dispatch did not mark artifact on loan: %#v", loadedArtifact)
	}
	if _, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "borrower", ToHolder: "courier", Location: "return dock", SealNumber: "SEAL-1", Kind: "returning"}); err != nil {
		t.Fatal(err)
	}
	returned, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "courier", ToHolder: "registrar", Location: "assessment room", SealNumber: "SEAL-2", Kind: "returned"})
	if err != nil {
		t.Fatal(err)
	}
	if returned.Kind != "returned" {
		t.Fatalf("return custody wrong: %#v", returned)
	}
	loadedArtifact, err = store.GetArtifact(context.Background(), artifact.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedArtifact.Status != domain.ArtifactAssessment || loadedArtifact.ActiveLoanID != "" || loadedArtifact.CurrentZoneID != "assessment room" {
		t.Fatalf("returned artifact state wrong: %#v", loadedArtifact)
	}
}

func TestLoanRejectsInvalidWindowsOverlapsAndEligibilityConflicts(t *testing.T) {
	store := loanStore(t)
	registrar := loanUser(t, store, "registrar", domain.RoleRegistrar)
	artifact := loanArtifact(t, store, "artifact-loan")
	service := loanService(store)
	start, end := loanPeriod(store)
	tests := []CreateInput{
		{ArtifactID: artifact.ID, Borrower: "Borrower", Purpose: "Purpose", StartAt: store.Now().Add(-time.Hour), EndAt: end},
		{ArtifactID: artifact.ID, Borrower: "Borrower", Purpose: "Purpose", StartAt: end, EndAt: start},
		{ArtifactID: artifact.ID, Borrower: "", Purpose: "Purpose", StartAt: start, EndAt: end},
	}
	for _, input := range tests {
		if _, err := service.Create(loanContext(registrar), input); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("invalid loan input should fail: %v", err)
		}
	}
	first, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum A", Purpose: "Exhibition A", StartAt: start, EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Submit(loanContext(registrar), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum B", Purpose: "Exhibition B", StartAt: start.Add(time.Hour), EndAt: end.Add(time.Hour)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("overlapping commitment should fail after submission, got %v", err)
	}
	supervisor := loanUser(t, store, "supervisor", domain.RoleSupervisor)
	if _, err := service.Approve(loanContext(supervisor), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum C", Purpose: "Exhibition C", StartAt: end.Add(time.Hour), EndAt: end.Add(2 * time.Hour)}); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("active loan should make artifact ineligible, got %v", err)
	}
}

func TestLoanApprovalBlocksActiveIncidentAndWrongRoles(t *testing.T) {
	store := loanStore(t)
	registrar := loanUser(t, store, "registrar", domain.RoleRegistrar)
	coordinator := loanUser(t, store, "coordinator", domain.RoleCoordinator)
	supervisor := loanUser(t, store, "supervisor", domain.RoleSupervisor)
	artifact := loanArtifact(t, store, "artifact-loan")
	service := loanService(store)
	start, end := loanPeriod(store)
	item, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum", Purpose: "Exhibition", StartAt: start, EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Submit(loanContext(registrar), item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(loanContext(coordinator), item.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("coordinator should not approve loan, got %v", err)
	}
	if _, err := store.DB.Exec(`INSERT INTO incidents(id, tenant_id, display_case_id, artifact_id, window_key, kind, status, summary, remediated, version, opened_at, updated_at)
		VALUES ('incident-loan', 'museum-demo', 'case-east-01', ?, 'window-1', 'humidity', 'open', 'unsafe', 0, 1, ?, ?)`, artifact.ID, store.Now().Format(time.RFC3339Nano), store.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(loanContext(supervisor), item.ID); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("active incident should block approval, got %v", err)
	}
	if _, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum B", Purpose: "Exhibition", StartAt: start, EndAt: end}); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("active incident should block a new application, got %v", err)
	}
}

func TestCustodyTransitionRequiresOrderedPhasesAndRequiredLocation(t *testing.T) {
	store := loanStore(t)
	coordinator := loanUser(t, store, "coordinator", domain.RoleCoordinator)
	artifact := loanArtifact(t, store, "artifact-loan")
	service := loanService(store)
	start, end := loanPeriod(store)
	// Seed a draft owned by the coordinator so the transition helper can be exercised through the service boundary.
	item := domain.LoanRequest{ID: "loan-draft", TenantID: artifact.TenantID, ArtifactID: artifact.ID, Borrower: "Museum", Purpose: "Exhibition", StartAt: start, EndAt: end, Status: domain.LoanDraft, Version: 1, CreatedBy: coordinator.ID, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if err := store.CreateLoan(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "a", ToHolder: "b", Location: "c", Kind: "dispatched"}); !errors.Is(err, domain.ErrIllegalState) {
		t.Fatalf("dispatch before approval should fail, got %v", err)
	}
	if _, err := store.DB.Exec(`UPDATE loan_requests SET status = 'approved' WHERE id = ?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustody(loanContext(coordinator), CustodyInput{LoanID: item.ID, FromHolder: "a", ToHolder: "b", Location: "", Kind: "packed"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("missing location should fail, got %v", err)
	}
}
