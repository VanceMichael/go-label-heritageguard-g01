package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type task0003Users struct{}

func (task0003Users) CreateUser(context.Context, domain.User) error { return nil }
func (task0003Users) FindUserByEmail(context.Context, string, string) (domain.User, error) {
	return domain.User{}, fmt.Errorf("sqlite connection lost: %w", domain.ErrUnavailable)
}
func (task0003Users) FindUser(context.Context, string, string) (domain.User, error) { return domain.User{}, domain.ErrNotFound }
func (task0003Users) SetUserActive(context.Context, string, string, bool, int64) error { return nil }
func (task0003Users) ListActiveSessions(context.Context, string, string) ([]domain.Session, error) { return nil, nil }

type task0003Sessions struct{}

func (task0003Sessions) CreateSession(context.Context, domain.Session) error { return nil }
func (task0003Sessions) FindSessionByToken(context.Context, []byte) (domain.Session, error) { return domain.Session{}, nil }
func (task0003Sessions) RevokeSession(context.Context, string, string, time.Time) error { return nil }
func (task0003Sessions) RevokeUserSessions(context.Context, string, string, time.Time) error { return nil }

func TestHeritageGuardTask0003(t *testing.T) {
	service := &Service{Users: task0003Users{}, Sessions: task0003Sessions{}, SessionTTL: time.Hour, Now: time.Now}
	_, err := service.Login(context.Background(), LoginInput{TenantID: "museum-demo", Email: "staff@museum.invalid", Password: "a-valid-password"})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("storage outage must remain distinguishable from bad credentials, got %v", err)
	}
}
