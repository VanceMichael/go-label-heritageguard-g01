package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type task0002Users struct{ observed context.Context }

func (r *task0002Users) CreateUser(context.Context, domain.User) error { return nil }
func (r *task0002Users) FindUserByEmail(context.Context, string, string) (domain.User, error) { return domain.User{}, domain.ErrNotFound }
func (r *task0002Users) FindUser(ctx context.Context, _, _ string) (domain.User, error) {
	r.observed = ctx
	return domain.User{ID: "user-2", TenantID: "museum-demo", Role: domain.RoleRegistrar, Active: true}, nil
}
func (r *task0002Users) SetUserActive(context.Context, string, string, bool, int64) error { return nil }
func (r *task0002Users) ListActiveSessions(context.Context, string, string) ([]domain.Session, error) { return nil, nil }

type task0002Sessions struct{ cancel context.CancelFunc }

func (task0002Sessions) CreateSession(context.Context, domain.Session) error { return nil }
func (s task0002Sessions) FindSessionByToken(context.Context, []byte) (domain.Session, error) {
	s.cancel()
	return domain.Session{ID: "session-2", TenantID: "museum-demo", UserID: "user-2", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (task0002Sessions) RevokeSession(context.Context, string, string, time.Time) error { return nil }
func (task0002Sessions) RevokeUserSessions(context.Context, string, string, time.Time) error { return nil }

func TestHeritageGuardTask0002(t *testing.T) {
	users := &task0002Users{}
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{Users: users, Sessions: task0002Sessions{cancel: cancel}, Now: time.Now}
	if _, err := service.Authenticate(ctx, "valid-token"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled authentication must stop before loading the user, got %v", err)
	}
	if users.observed != nil {
		t.Fatalf("user repository was reached after cancellation with context %v", users.observed)
	}
}
