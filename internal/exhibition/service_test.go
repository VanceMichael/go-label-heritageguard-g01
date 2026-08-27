package exhibition

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/eventbus"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
)

type exhibitionLogWriter struct{}

func (exhibitionLogWriter) Write(p []byte) (int, error) { return len(p), nil }

type exhibitionIDs struct{ index int }

func (g *exhibitionIDs) New(prefix string) string {
	g.index++
	return prefix + "-" + string(rune('a'+g.index))
}

func exhibitionStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(context.Background(), ":memory:", 1, slog.New(slog.NewTextHandler(exhibitionLogWriter{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func exhibitionUser(t *testing.T, store *sqlite.Store, id string, role domain.Role) domain.User {
	t.Helper()
	user := domain.User{ID: id, TenantID: "museum-demo", Email: id + "@museum.invalid", DisplayName: id,
		Role: role, PasswordHash: []byte("hash"), Active: true, Version: 1, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

func exhibitionArtifact(t *testing.T, store *sqlite.Store, id string) domain.Artifact {
	t.Helper()
	artifact := domain.Artifact{ID: id, TenantID: "museum-demo", AccessionNo: id, Name: "Bronze bell", Material: "bronze",
		Period: "Han", RiskClass: domain.RiskLow, Status: domain.ArtifactReady, CurrentZoneID: "zone-general", Version: 1,
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

func exhibitionService(store *sqlite.Store) *Service {
	return &Service{Artifacts: store, Cases: store, IDs: &exhibitionIDs{}, Now: store.Now}
}

func exhibitionContext(user domain.User) context.Context {
	return service.WithPrincipal(service.WithRequestID(context.Background(), "exhibition-request"), domain.Principal{TenantID: user.TenantID, UserID: user.ID, Role: user.Role})
}

func TestReserveAndActivateDisplayCaseUsesVersionedAtomicWrites(t *testing.T) {
	store := exhibitionStore(t)
	coordinator := exhibitionUser(t, store, "coordinator", domain.RoleCoordinator)
	artifact := exhibitionArtifact(t, store, "artifact-display")
	service := exhibitionService(store)
	reserved, err := service.Reserve(exhibitionContext(coordinator), ReservationInput{ArtifactID: artifact.ID, DisplayCaseID: "case-east-01", Duration: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if reserved.Status != domain.CaseReserved || reserved.ArtifactID != artifact.ID || reserved.ReservationTo == nil {
		t.Fatalf("reservation wrong: %#v", reserved)
	}
	installation, err := service.Activate(exhibitionContext(coordinator), InstallationInput{ArtifactID: artifact.ID, DisplayCaseID: reserved.ID, MountVerified: true, SealVerified: true, EnvironmentReady: true})
	if err != nil {
		t.Fatal(err)
	}
	if installation.ID == "" || !installation.Complete() {
		t.Fatalf("installation wrong: %#v", installation)
	}
	loadedCase, err := store.GetDisplayCase(context.Background(), coordinator.TenantID, reserved.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedArtifact, err := store.GetArtifact(context.Background(), artifact.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedCase.Status != domain.CaseActive || loadedArtifact.Status != domain.ArtifactOnDisplay || loadedArtifact.CurrentCaseID != reserved.ID {
		t.Fatalf("display activation inconsistent: case=%#v artifact=%#v", loadedCase, loadedArtifact)
	}
}

func TestDisplayReservationRejectsUnavailableArtifactCaseAndChecklist(t *testing.T) {
	store := exhibitionStore(t)
	coordinator := exhibitionUser(t, store, "coordinator", domain.RoleCoordinator)
	artifact := exhibitionArtifact(t, store, "artifact-display")
	service := exhibitionService(store)
	if _, err := service.Reserve(exhibitionContext(coordinator), ReservationInput{ArtifactID: artifact.ID, DisplayCaseID: "case-east-01", Duration: 73 * time.Hour}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected reservation duration validation, got %v", err)
	}
	if _, err := service.Activate(exhibitionContext(coordinator), InstallationInput{ArtifactID: artifact.ID, DisplayCaseID: "case-east-01", MountVerified: true, SealVerified: true, EnvironmentReady: true}); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("unreserved case should not activate, got %v", err)
	}
	if _, err := service.Reserve(exhibitionContext(coordinator), ReservationInput{ArtifactID: artifact.ID, DisplayCaseID: "case-east-01", Duration: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(exhibitionContext(coordinator), InstallationInput{ArtifactID: artifact.ID, DisplayCaseID: "case-east-01", MountVerified: true, SealVerified: true}); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("incomplete checklist should fail, got %v", err)
	}
}

func TestEnvironmentReadingEnqueuesAssessmentAndOpensIncident(t *testing.T) {
	store := exhibitionStore(t)
	service := exhibitionService(store)
	now := store.Now()
	for index, humidity := range []float64{70, 71, 72} {
		_, err := service.RecordReading(context.Background(), "museum-demo", ReadingInput{
			DisplayCaseID: "case-east-01", DeviceID: "device-1", Sequence: int64(index + 1), TemperatureC: 20,
			Humidity: humidity, ObservedAt: now.Add(time.Duration(index) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var jobs int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM worker_jobs WHERE kind = ?`, EnvironmentAssessmentJob).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 3 {
		t.Fatalf("expected one assessment job per reading, got %d", jobs)
	}
	job, err := store.ClaimJob(context.Background(), "test-worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessAssessmentJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var incidentCount, outboxCount int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM incidents`).Scan(&incidentCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM outbox_events`).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if incidentCount != 1 || outboxCount != 1 {
		t.Fatalf("assessment did not atomically create alert/outbox: incidents=%d outbox=%d", incidentCount, outboxCount)
	}
	if err := store.CompleteJob(context.Background(), job.ID, "test-worker", now); err != nil {
		t.Fatal(err)
	}
}

func TestIncidentOpeningIsIdempotentAndOutboxIsNotDuplicated(t *testing.T) {
	store := exhibitionStore(t)
	service := exhibitionService(store)
	input := IncidentInput{DisplayCaseID: "case-east-01", WindowKey: "2026-08-22T12:00:00Z", Kind: "humidity", Summary: "humidity high"}
	first, err := service.OpenIncident(context.Background(), "museum-demo", input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.OpenIncident(context.Background(), "museum-demo", input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate incident created: %s vs %s", first.ID, second.ID)
	}
	var incidents, events int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM incidents`).Scan(&incidents); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM outbox_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if incidents != 1 || events != 1 {
		t.Fatalf("idempotency counts wrong: incidents=%d events=%d", incidents, events)
	}
}

func TestIncidentNotificationUsesCancellableEventBusOnlyForNewIncident(t *testing.T) {
	store := exhibitionStore(t)
	bus := eventbus.New()
	defer bus.Close()
	subscription, err := bus.Subscribe("exhibition.incident.opened", 1)
	if err != nil {
		t.Fatal(err)
	}
	service := exhibitionService(store)
	service.Events = bus
	input := IncidentInput{DisplayCaseID: "case-east-01", WindowKey: "window-bus", Kind: "humidity", Summary: "humidity high"}
	first, err := service.OpenIncident(context.Background(), "museum-demo", input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-subscription.Events:
		if event.Key != first.ID || event.Topic != "exhibition.incident.opened" {
			t.Fatalf("unexpected incident event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("new incident notification was not published")
	}
	if _, err := service.OpenIncident(context.Background(), "museum-demo", input); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-subscription.Events:
		t.Fatalf("duplicate incident emitted a second event: %#v", event)
	case <-time.After(20 * time.Millisecond):
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.OpenIncident(ctx, "museum-demo", IncidentInput{DisplayCaseID: "case-east-01", WindowKey: "window-cancel", Kind: "temperature", Summary: "temperature high"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled event publish should be reported: %v", err)
	}
}

func TestAssessmentRequiresAuthenticatedRoleAndValidWindow(t *testing.T) {
	store := exhibitionStore(t)
	registrar := exhibitionUser(t, store, "registrar", domain.RoleRegistrar)
	service := exhibitionService(store)
	if _, err := service.Assess(exhibitionContext(registrar), "case-east-01", time.Minute); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("registrar should not assess environment, got %v", err)
	}
	coordinator := exhibitionUser(t, store, "coordinator", domain.RoleCoordinator)
	if _, err := service.Assess(exhibitionContext(coordinator), "case-east-01", time.Minute); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short assessment window should fail, got %v", err)
	}
	assessment, err := service.Assess(exhibitionContext(coordinator), "case-east-01", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Ready || assessment.ReadingCount != 0 {
		t.Fatalf("missing readings should produce an unready assessment: %#v", assessment)
	}
}

func TestAssessmentJobPayloadRemainsStructured(t *testing.T) {
	store := exhibitionStore(t)
	service := exhibitionService(store)
	_, err := service.RecordReading(context.Background(), "museum-demo", ReadingInput{DisplayCaseID: "case-east-01", DeviceID: "device-structured", Sequence: 1, TemperatureC: 20, Humidity: 50, ObservedAt: store.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	if err := store.DB.QueryRow(`SELECT payload FROM worker_jobs WHERE kind = ?`, EnvironmentAssessmentJob).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["display_case_id"] != "case-east-01" || decoded["window"] != "15m" {
		t.Fatalf("assessment job payload incomplete: %#v", decoded)
	}
}
