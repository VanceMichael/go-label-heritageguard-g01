package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

// stubUserRepo is a minimal UserRepository whose lookup methods return a
// configured error so the auth service can exercise its dependency-failure
// and not-found propagation paths.
type stubUserRepo struct {
	findByEmailErr error
	findErr        error
	user           domain.User
}

func (r *stubUserRepo) CreateUser(context.Context, domain.User) error {
	return nil
}

func (r *stubUserRepo) FindUserByEmail(_ context.Context, _, email string) (domain.User, error) {
	if r.findByEmailErr != nil {
		return domain.User{}, r.findByEmailErr
	}
	if r.user.Email != "" && r.user.Email != email {
		return domain.User{}, domain.ErrNotFound
	}
	return r.user, nil
}

func (r *stubUserRepo) FindUser(context.Context, string, string) (domain.User, error) {
	if r.findErr != nil {
		return domain.User{}, r.findErr
	}
	return r.user, nil
}

func (r *stubUserRepo) SetUserActive(context.Context, string, string, bool, int64) error {
	return nil
}

func (r *stubUserRepo) ListActiveSessions(context.Context, string, string) ([]domain.Session, error) {
	return nil, nil
}

// stubSessionRepo lets the auth service observe session-store failures during
// login (CreateSession) and authentication (FindSessionByToken).
type stubSessionRepo struct {
	createErr      error
	findByTokenErr error
	session        domain.Session
}

func (r *stubSessionRepo) CreateSession(context.Context, domain.Session) error {
	return r.createErr
}

func (r *stubSessionRepo) FindSessionByToken(context.Context, []byte) (domain.Session, error) {
	if r.findByTokenErr != nil {
		return domain.Session{}, r.findByTokenErr
	}
	return r.session, nil
}

func (r *stubSessionRepo) RevokeSession(context.Context, string, string, time.Time) error {
	return nil
}

func (r *stubSessionRepo) RevokeUserSessions(context.Context, string, string, time.Time) error {
	return nil
}

// failingTokens is a TokenGenerator that always fails with a dependency error,
// modelling an unavailable login dependency (e.g. the CSPRNG source).
type failingTokens struct{}

func (failingTokens) NewToken() (string, error) {
	return "", domain.DependencyError{Operation: "generate session token", Err: errors.New("entropy source offline")}
}

func newServiceWith(users repository.UserRepository, sessions repository.SessionRepository) *Service {
	return &Service{
		Users: users, Sessions: sessions, Deactivator: nil,
		IDs: &authIDs{}, Tokens: &authTokens{}, SessionTTL: 2 * time.Hour, Now: func() time.Time {
			return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
		},
	}
}

func TestLoginDependencyFailurePropagatesAsUnavailable(t *testing.T) {
	hash, err := HashPassword("a-valid-password")
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{
		ID: "registrar", TenantID: "museum-demo", Email: "registrar@museum.invalid",
		DisplayName: "registrar", Role: domain.RoleRegistrar, PasswordHash: hash,
		Active: true, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	dep := domain.DependencyError{Operation: "load login user", Err: errors.New("database offline")}
	tests := []struct {
		name    string
		users   repository.UserRepository
		sessions repository.SessionRepository
		tokens  service.TokenGenerator
		want    error
	}{
		{
			name:    "user store unavailable",
			users:   &stubUserRepo{findByEmailErr: dep, user: user},
			sessions: &stubSessionRepo{},
			tokens:  &authTokens{},
			want:    domain.ErrUnavailable,
		},
		{
			name:    "token generator unavailable",
			users:   &stubUserRepo{user: user},
			sessions: &stubSessionRepo{},
			tokens:  failingTokens{},
			want:    domain.ErrUnavailable,
		},
		{
			name:    "session store unavailable",
			users:   &stubUserRepo{user: user},
			sessions: &stubSessionRepo{createErr: domain.DependencyError{Operation: "persist login session", Err: errors.New("write failed")}},
			tokens:  &authTokens{},
			want:    domain.ErrUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := newServiceWith(test.users, test.sessions)
			svc.Tokens = test.tokens
			_, err := svc.Login(context.Background(), LoginInput{
				TenantID: user.TenantID, Email: user.Email, Password: "a-valid-password",
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestLoginCredentialSemanticsArePreserved(t *testing.T) {
	hash, err := HashPassword("a-valid-password")
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{
		ID: "registrar", TenantID: "museum-demo", Email: "registrar@museum.invalid",
		DisplayName: "registrar", Role: domain.RoleRegistrar, PasswordHash: hash,
		Active: true, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	svc := newServiceWith(&stubUserRepo{user: user}, &stubSessionRepo{})
	if _, err := svc.Login(context.Background(), LoginInput{
		TenantID: user.TenantID, Email: "missing@museum.invalid", Password: "a-valid-password",
	}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("missing user should be unauthorized, got %v", err)
	}
	if _, err := svc.Login(context.Background(), LoginInput{
		TenantID: user.TenantID, Email: user.Email, Password: "wrong-password",
	}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password should be unauthorized, got %v", err)
	}
}

func TestAuthenticateDependencyFailurePropagatesAsUnavailable(t *testing.T) {
	dep := domain.DependencyError{Operation: "find session", Err: errors.New("database offline")}
	svc := newServiceWith(&stubUserRepo{}, &stubSessionRepo{findByTokenErr: dep})
	if _, err := svc.Authenticate(context.Background(), "token"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("session store unavailable should surface as dependency failure, got %v", err)
	}
	user := domain.User{
		ID: "registrar", TenantID: "museum-demo", Role: domain.RoleRegistrar, Active: true,
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	session := domain.Session{
		ID: "session", TenantID: "museum-demo", UserID: "registrar",
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	svc = newServiceWith(&stubUserRepo{findErr: domain.DependencyError{Operation: "find session user", Err: errors.New("database offline")}, user: user}, &stubSessionRepo{session: session})
	if _, err := svc.Authenticate(context.Background(), "token"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("user store unavailable during authenticate should surface as dependency failure, got %v", err)
	}
}

func TestAuthenticateExpiredAndRevokedSemanticsArePreserved(t *testing.T) {
	user := domain.User{
		ID: "registrar", TenantID: "museum-demo", Role: domain.RoleRegistrar, Active: true,
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	expired := domain.Session{
		ID: "session", TenantID: "museum-demo", UserID: "registrar",
		ExpiresAt: now.Add(-time.Hour), CreatedAt: now.Add(-2 * time.Hour),
	}
	svc := newServiceWith(&stubUserRepo{user: user}, &stubSessionRepo{session: expired})
	svc.Now = func() time.Time { return now }
	if _, err := svc.Authenticate(context.Background(), "token"); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("expired session should remain expired, got %v", err)
	}
	revoked := domain.Session{
		ID: "session", TenantID: "museum-demo", UserID: "registrar",
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	revokedAt := now.Add(-time.Minute)
	revoked.RevokedAt = &revokedAt
	svc = newServiceWith(&stubUserRepo{user: user}, &stubSessionRepo{session: revoked})
	svc.Now = func() time.Time { return now }
	if _, err := svc.Authenticate(context.Background(), "token"); !errors.Is(err, domain.ErrRevoked) {
		t.Fatalf("revoked session should remain revoked, got %v", err)
	}
	// A session whose user has since been removed must surface as unauthorized
	// rather than as a not-found leak.
	active := domain.Session{
		ID: "session", TenantID: "museum-demo", UserID: "registrar",
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	svc = newServiceWith(&stubUserRepo{findErr: domain.ErrNotFound, user: user}, &stubSessionRepo{session: active})
	svc.Now = func() time.Time { return now }
	if _, err := svc.Authenticate(context.Background(), "token"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("missing session user should be unauthorized, got %v", err)
	}
}
