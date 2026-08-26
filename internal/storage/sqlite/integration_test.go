package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/auth"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func integrationNow() time.Time {
	return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
}

func integrationStore(t *testing.T, path string, maxOpen int) *Store {
	t.Helper()
	store, err := Open(context.Background(), path, maxOpen, slog.New(slog.NewTextHandler(testOutput{t}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	store.Now = integrationNow
	t.Cleanup(func() { _ = store.Close() })
	return store
}

type testOutput struct{ t *testing.T }

func (w testOutput) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func fixtureUser(t *testing.T, store *Store, id string, role domain.Role) domain.User {
	t.Helper()
	user := domain.User{
		ID: id, TenantID: "museum-demo", Email: id + "@museum.invalid", DisplayName: id,
		Role: role, PasswordHash: []byte("password-hash"), Active: true, Version: 1,
		CreatedAt: integrationNow(), UpdatedAt: integrationNow(),
	}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

func fixtureArtifact(t *testing.T, store *Store, id string, status domain.ArtifactStatus) domain.Artifact {
	t.Helper()
	artifact := domain.Artifact{
		ID: id, TenantID: "museum-demo", AccessionNo: id, Name: "Bronze vessel",
		Material: "bronze", Period: "Han", RiskClass: domain.RiskModerate,
		Status: status, CurrentZoneID: "zone-general", Version: 1,
		CreatedAt: integrationNow(), UpdatedAt: integrationNow(),
	}
	if _, err := store.DB.Exec(`INSERT INTO artifacts(
		id, tenant_id, accession_no, name, material, period, risk_class, status,
		current_zone_id, current_case_id, active_loan_id, last_report_id, version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?, ?)`, artifact.ID, artifact.TenantID,
		artifact.AccessionNo, artifact.Name, artifact.Material, artifact.Period, artifact.RiskClass,
		artifact.Status, artifact.CurrentZoneID, artifact.Version, timeText(artifact.CreatedAt), timeText(artifact.UpdatedAt)); err != nil {
		t.Fatal(err)
	}
	return artifact
}

func TestMigrationsAreIdempotentAndRecoverAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "heritageguard.db")
	store := integrationStore(t, path, 4)
	var migrationCount int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != 2 {
		t.Fatalf("expected two migrations, got %d", migrationCount)
	}
	fixtureUser(t, store, "supervisor-1", domain.RoleSupervisor)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path, 4, slog.New(slog.NewTextHandler(testOutput{t}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	reopened.Now = integrationNow
	defer reopened.Close()
	user, err := reopened.FindUser(context.Background(), "museum-demo", "supervisor-1")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != domain.RoleSupervisor || !user.Active {
		t.Fatalf("reopened user is wrong: %#v", user)
	}
}

func TestTransactionRollsBackAllArtifactRegistrationWrites(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	user := fixtureUser(t, store, "registrar-1", domain.RoleRegistrar)
	artifact := domain.Artifact{
		ID: "artifact-rollback", TenantID: user.TenantID, AccessionNo: "ROLLBACK-1", Name: "Artifact",
		Material: "paper", Period: "Qing", RiskClass: domain.RiskLow, Status: domain.ArtifactRegistered,
		CurrentZoneID: "zone-general", Version: 1, CreatedAt: integrationNow(), UpdatedAt: integrationNow(),
	}
	report := domain.ConditionReport{ID: "report-rollback", TenantID: user.TenantID, ArtifactID: artifact.ID,
		InspectorID: user.ID, Summary: "intake", Severity: domain.RiskLow, Measurements: map[string]float64{},
		ObservedIssues: []string{}, Final: true, CreatedAt: integrationNow()}
	custody := domain.CustodyEvent{ID: "custody-rollback", TenantID: user.TenantID, ArtifactID: artifact.ID,
		FromHolder: "outside", ToHolder: user.ID, Location: "zone-general", Kind: "registered",
		OccurredAt: integrationNow(), RecordedBy: user.ID}
	audit := domain.AuditEvent{ID: "audit-rollback", TenantID: user.TenantID, ActorID: user.ID, Action: "artifact.register", ObjectType: "artifact", ObjectID: artifact.ID,
		Result: "success", RequestID: "request", Details: []byte(`{"initial":true}`), CreatedAt: integrationNow()}
	if err := store.CreateArtifact(context.Background(), artifact, report, custody, audit); err != nil {
		t.Fatal(err)
	}
	duplicate := artifact
	duplicate.ID = "artifact-duplicate"
	duplicate.AccessionNo = "ROLLBACK-2"
	duplicateReport := report
	duplicateReport.ID = report.ID
	duplicateReport.ArtifactID = duplicate.ID
	duplicateCustody := custody
	duplicateCustody.ID = custody.ID
	duplicateCustody.ArtifactID = duplicate.ID
	duplicateAudit := audit
	duplicateAudit.ObjectID = duplicate.ID
	if err := store.CreateArtifact(context.Background(), duplicate, duplicateReport, duplicateCustody, duplicateAudit); err == nil {
		t.Fatal("expected duplicate condition report to abort transaction")
	}
	var artifactCount, reportCount, custodyCount int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE id = ?`, duplicate.ID).Scan(&artifactCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM condition_reports WHERE id = ?`, duplicateReport.ID).Scan(&reportCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM custody_events WHERE id = ?`, duplicateCustody.ID).Scan(&custodyCount); err != nil {
		t.Fatal(err)
	}
	if artifactCount != 0 || reportCount != 1 || custodyCount != 1 {
		t.Fatalf("transaction left partial writes: artifacts=%d reports=%d custody=%d", artifactCount, reportCount, custodyCount)
	}
}

func TestOptimisticVersionPreventsLostArtifactUpdate(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	artifact := fixtureArtifact(t, store, "artifact-version", domain.ArtifactReady)
	if err := store.UpdateArtifactStatus(context.Background(), artifact.TenantID, artifact.ID, domain.ArtifactOnDisplay, artifact.Version); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateArtifactStatus(context.Background(), artifact.TenantID, artifact.ID, domain.ArtifactOnLoan, artifact.Version); !errors.Is(err, domain.ErrVersion) {
		t.Fatalf("expected stale version conflict, got %v", err)
	}
	current, err := store.GetArtifact(context.Background(), artifact.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.ArtifactOnDisplay || current.Version != 2 {
		t.Fatalf("stale update changed state: %#v", current)
	}
}

func TestQuarantineCapacityIsAtomic(t *testing.T) {
	store := integrationStore(t, ":memory:", 4)
	conservator := fixtureUser(t, store, "conservator-1", domain.RoleConservator)
	first := fixtureArtifact(t, store, "artifact-q-1", domain.ArtifactReady)
	second := fixtureArtifact(t, store, "artifact-q-2", domain.ArtifactReady)
	if _, err := store.DB.Exec(`UPDATE quarantine_zones SET capacity = 1 WHERE id = 'zone-general'`); err != nil {
		t.Fatal(err)
	}
	makeCase := func(id string, artifact domain.Artifact) domain.QuarantineCase {
		return domain.QuarantineCase{ID: id, TenantID: artifact.TenantID, ArtifactID: artifact.ID, ZoneID: "zone-general",
			Reason: "humidity spike", Status: domain.QuarantineOpen, OpenedBy: conservator.ID, Version: 1, OpenedAt: integrationNow()}
	}
	if err := store.OpenQuarantine(context.Background(), makeCase("q-1", first), first, first.Version); err != nil {
		t.Fatal(err)
	}
	if err := store.OpenQuarantine(context.Background(), makeCase("q-2", second), second, second.Version); !errors.Is(err, domain.ErrCapacity) {
		t.Fatalf("expected capacity failure, got %v", err)
	}
	var occupied int
	if err := store.DB.QueryRow(`SELECT occupied FROM quarantine_zones WHERE id = 'zone-general'`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if occupied != 1 {
		t.Fatalf("capacity was corrupted after rejected quarantine: %d", occupied)
	}
	current, err := store.GetArtifact(context.Background(), second.TenantID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.ArtifactReady {
		t.Fatalf("rejected quarantine moved artifact: %#v", current)
	}
}

func TestWorkerClaimIsSingleOwnerUnderConcurrency(t *testing.T) {
	store := integrationStore(t, filepath.Join(t.TempDir(), "claim.db"), 8)
	now := integrationNow()
	job := domain.WorkerJob{ID: "job-claim", TenantID: "museum-demo", Kind: "assessment", AggregateID: "case-1",
		Payload: []byte(`{"case":"case-1"}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now,
		CreatedAt: now, UpdatedAt: now}
	if err := store.EnqueueJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan domain.WorkerJob, 2)
	errorsCh := make(chan error, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		owner := owner
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := store.ClaimJob(context.Background(), owner, now, time.Minute)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- claimed
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	var claimed []domain.WorkerJob
	for item := range results {
		claimed = append(claimed, item)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one owner, got %#v errors=%v", claimed, errorsCh)
	}
	if claimed[0].Attempts != 1 || claimed[0].LeaseOwner == "" {
		t.Fatalf("claim did not establish lease: %#v", claimed[0])
	}
}

func TestOutboxLeaseFailureRetryAndSuccessSurvivePersistence(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	now := integrationNow()
	event := domain.OutboxEvent{ID: "outbox-1", TenantID: "museum-demo", Topic: "incident.opened", AggregateID: "incident-1",
		IdempotencyKey: "incident-1-opened", Payload: []byte(`{"incident":"incident-1"}`), Status: domain.JobPending,
		MaxAttempts: 2, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.EnqueueOutbox(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimOutbox(context.Background(), "dispatcher-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Attempts != 1 || claimed.LeaseOwner != "dispatcher-a" || claimed.Status != domain.JobRunning {
		t.Fatalf("outbox claim wrong: %#v", claimed)
	}
	if err := store.FinishOutbox(context.Background(), claimed.ID, claimed.LeaseOwner, now, errors.New("remote unavailable")); err != nil {
		t.Fatal(err)
	}
	retryNow := now.Add(2 * time.Second)
	claimed, err = store.ClaimOutbox(context.Background(), "dispatcher-b", retryNow, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Attempts != 2 || claimed.LeaseOwner != "dispatcher-b" {
		t.Fatalf("outbox retry did not reclaim event: %#v", claimed)
	}
	if err := store.FinishOutbox(context.Background(), claimed.ID, claimed.LeaseOwner, retryNow, nil); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := store.DB.QueryRow(`SELECT status FROM outbox_events WHERE id = ?`, event.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.JobSucceeded) {
		t.Fatalf("outbox did not persist success: %s", status)
	}
}

func TestCancelledTransactionDoesNotCommit(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := store.WithTx(ctx, func(context.Context, *sql.Tx) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled transaction, got %v", err)
	}
}

func TestCredentialAndTokenPersistence(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	passwordHash, err := auth.HashPassword("a-strong-password")
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{ID: "user-auth", TenantID: "museum-demo", Email: "auth@museum.invalid", DisplayName: "Auth",
		Role: domain.RoleSupervisor, PasswordHash: passwordHash, Active: true, Version: 1,
		CreatedAt: integrationNow(), UpdatedAt: integrationNow()}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindUserByEmail(context.Background(), user.TenantID, user.Email); err != nil {
		t.Fatal(err)
	}
	token := auth.HashToken("secret-token")
	session := domain.Session{ID: "session-auth", TenantID: user.TenantID, UserID: user.ID, TokenHash: token,
		ExpiresAt: integrationNow().Add(time.Hour), CreatedAt: integrationNow()}
	if err := store.CreateSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.FindSessionByToken(context.Background(), token)
	if err != nil || loaded.ID != session.ID {
		t.Fatalf("session did not persist: %#v %v", loaded, err)
	}
	if err := store.RevokeSession(context.Background(), user.TenantID, session.ID, integrationNow()); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.FindSessionByToken(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RevokedAt == nil {
		t.Fatal("revoked session lost revocation timestamp")
	}
}

func TestDeactivateUserAndSessionsRollsBackWhenAuditWriteFails(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	supervisor := fixtureUser(t, store, "supervisor-deact", domain.RoleSupervisor)
	target := fixtureUser(t, store, "target-deact", domain.RoleRegistrar)
	session := domain.Session{ID: "session-deact", TenantID: target.TenantID, UserID: target.ID,
		TokenHash: auth.HashToken("target-session"), ExpiresAt: integrationNow().Add(time.Hour), CreatedAt: integrationNow()}
	if err := store.CreateSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`CREATE TRIGGER audit_block BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	actor := domain.Principal{TenantID: supervisor.TenantID, UserID: supervisor.ID, Role: supervisor.Role}
	err := store.DeactivateUserAndSessions(context.Background(), actor, target, "request-deact")
	if err == nil {
		t.Fatal("expected audit failure to abort deactivation")
	}
	if !containsAny(err.Error(), "audit unavailable", "insert audit event") {
		t.Fatalf("expected audit write error, got %v", err)
	}
	loaded, err := store.FindUser(context.Background(), target.TenantID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Active || loaded.Version != target.Version {
		t.Fatalf("user was left half-deactivated: active=%v version=%d", loaded.Active, loaded.Version)
	}
	sessions, err := store.ListActiveSessions(context.Background(), target.TenantID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("target session was left half-revoked: %#v", sessions)
	}
	audit, err := store.List(context.Background(), target.TenantID, target.ID, domain.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 0 {
		t.Fatalf("deactivation audit should not persist on failure: %#v", audit)
	}
}
