package auth

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

func TestHeritageGuardTask0001(t *testing.T) {
	store := authStore(t)
	supervisor := seedAuthUser(t, store, "supervisor-task-0001", domain.RoleSupervisor, true)
	target := seedAuthUser(t, store, "target-task-0001", domain.RoleRegistrar, true)
	authService := newAuthService(store)
	login, err := authService.Login(context.Background(), LoginInput{TenantID: target.TenantID, Email: target.Email, Password: "a-valid-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(context.Background(), `CREATE TRIGGER fail_task_0001 AFTER INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	ctx := service.WithPrincipal(service.WithRequestID(context.Background(), "task-0001"), domain.Principal{TenantID: supervisor.TenantID, UserID: supervisor.ID, Role: supervisor.Role})
	if err := authService.DeactivateUser(ctx, target.ID); err == nil {
		t.Fatal("deactivation should report the injected audit failure")
	}
	loaded, err := store.FindUser(context.Background(), target.TenantID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Active {
		t.Fatal("failed deactivation must leave the user active")
	}
	if _, err := authService.Authenticate(context.Background(), login.Token); err != nil {
		t.Fatalf("failed deactivation must not revoke the session: %v", err)
	}
}
