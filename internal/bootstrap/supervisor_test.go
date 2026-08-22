package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
)

type bootstrapLogWriter struct{}

func (bootstrapLogWriter) Write(p []byte) (int, error) { return len(p), nil }

type bootstrapIDs struct{ index int }

func (g *bootstrapIDs) New(prefix string) string {
	g.index++
	return prefix + "-bootstrap-" + string(rune('a'+g.index))
}

func bootstrapStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(context.Background(), ":memory:", 1, slog.New(slog.NewTextHandler(bootstrapLogWriter{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestEnsureSupervisorCreatesOnlyWhenCredentialsAreConfigured(t *testing.T) {
	store := bootstrapStore(t)
	created, err := EnsureSupervisor(context.Background(), store, &bootstrapIDs{}, store.Now, SupervisorConfig{TenantID: "museum-demo"})
	if err != nil || created {
		t.Fatalf("empty bootstrap configuration should be a no-op: created=%v err=%v", created, err)
	}
	created, err = EnsureSupervisor(context.Background(), store, &bootstrapIDs{}, store.Now, SupervisorConfig{TenantID: "museum-demo", Email: "Supervisor@Museum.invalid", Name: "Supervisor", Password: "a-valid-password"})
	if err != nil || !created {
		t.Fatalf("configured bootstrap should create user: created=%v err=%v", created, err)
	}
	user, err := store.FindUserByEmail(context.Background(), "museum-demo", "supervisor@museum.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != domain.RoleSupervisor || !user.Active || len(user.PasswordHash) == 0 {
		t.Fatalf("bootstrap user wrong: %#v", user)
	}
	created, err = EnsureSupervisor(context.Background(), store, &bootstrapIDs{}, store.Now, SupervisorConfig{TenantID: "museum-demo", Email: "supervisor@museum.invalid", Name: "Supervisor", Password: "a-valid-password"})
	if err != nil || created {
		t.Fatalf("repeated bootstrap should be idempotent: created=%v err=%v", created, err)
	}
}

func TestEnsureSupervisorRejectsPartialOrUnsafeIdentity(t *testing.T) {
	store := bootstrapStore(t)
	ids := &bootstrapIDs{}
	for _, cfg := range []SupervisorConfig{
		{TenantID: "museum-demo", Email: "admin@museum.invalid"},
		{TenantID: "museum-demo", Password: "a-valid-password"},
		{TenantID: "museum-demo", Email: "admin@museum.invalid", Name: "", Password: "a-valid-password"},
	} {
		if _, err := EnsureSupervisor(context.Background(), store, ids, store.Now, cfg); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("expected invalid bootstrap config, got %v", err)
		}
	}
	created, err := EnsureSupervisor(context.Background(), store, ids, store.Now, SupervisorConfig{TenantID: "museum-demo", Email: "admin@museum.invalid", Name: "Admin", Password: "a-valid-password"})
	if err != nil || !created {
		t.Fatal(err)
	}
	if err := store.SetUserActive(context.Background(), "museum-demo", "user-bootstrap-b", false, 1); err == nil {
		t.Fatal("test fixture unexpectedly updated unknown bootstrap ID")
	}
}

func TestEnsureSupervisorDoesNotAcceptExistingNonSupervisor(t *testing.T) {
	store := bootstrapStore(t)
	user := domain.User{ID: "registrar", TenantID: "museum-demo", Email: "registrar@museum.invalid", DisplayName: "Registrar", Role: domain.RoleRegistrar,
		PasswordHash: []byte("hash"), Active: true, Version: 1, CreatedAt: store.Now(), UpdatedAt: store.Now()}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureSupervisor(context.Background(), store, &bootstrapIDs{}, store.Now, SupervisorConfig{TenantID: user.TenantID, Email: user.Email, Name: "Registrar", Password: "a-valid-password"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("existing non-supervisor should conflict: %v", err)
	}
}
