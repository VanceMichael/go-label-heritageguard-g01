package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type task0026Users struct {
	cancel context.CancelFunc
	created bool
}

func (s *task0026Users) CreateUser(ctx context.Context, _ domain.User) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.created = true
	return nil
}
func (s *task0026Users) FindUserByEmail(_ context.Context, _, _ string) (domain.User, error) { s.cancel(); return domain.User{}, domain.ErrNotFound }
func (s *task0026Users) FindUser(context.Context, string, string) (domain.User, error) { return domain.User{}, domain.ErrNotFound }
func (s *task0026Users) SetUserActive(context.Context, string, string, bool, int64) error { return nil }
func (s *task0026Users) ListActiveSessions(context.Context, string, string) ([]domain.Session, error) { return nil, nil }

func TestHeritageGuardTask0026(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	users := &task0026Users{cancel: cancel}
	_, err := EnsureSupervisor(ctx, users, &bootstrapIDs{}, time.Now, SupervisorConfig{TenantID: "museum-demo", Email: "supervisor@museum.invalid", Name: "Supervisor", Password: "a-valid-password"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled bootstrap must not create the supervisor: %v", err)
	}
	if users.created {
		t.Fatal("bootstrap reached persistence after cancellation")
	}
}
