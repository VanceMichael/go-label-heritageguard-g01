package exhibition

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0012(t *testing.T) {
	store := exhibitionStore(t)
	coordinator := exhibitionUser(t, store, "coordinator-task-0012", domain.RoleCoordinator)
	artifact := exhibitionArtifact(t, store, "artifact-task-0012")
	service := exhibitionService(store)
	reserved, err := service.Reserve(exhibitionContext(coordinator), ReservationInput{ArtifactID: artifact.ID, DisplayCaseID: "case-east-01", Duration: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0012 AFTER UPDATE ON artifacts WHEN NEW.status = 'on_display' BEGIN SELECT RAISE(ABORT, 'artifact activation failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(exhibitionContext(coordinator), InstallationInput{ArtifactID: artifact.ID, DisplayCaseID: reserved.ID, MountVerified: true, SealVerified: true, EnvironmentReady: true}); err == nil {
		t.Fatal("activation should report the artifact update failure")
	}
	var installations int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM installations WHERE artifact_id = ?`, artifact.ID).Scan(&installations); err != nil {
		t.Fatal(err)
	}
	loadedCase, err := store.GetDisplayCase(context.Background(), coordinator.TenantID, reserved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if installations != 0 || loadedCase.Status != domain.CaseReserved {
		t.Fatalf("failed activation left partial installation: installations=%d case=%s", installations, loadedCase.Status)
	}
}
