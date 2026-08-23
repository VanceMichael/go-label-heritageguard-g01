package auth

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
)

type authTestLogWriter struct{}

func (authTestLogWriter) Write(p []byte) (int, error) { return len(p), nil }

func authStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(context.Background(), ":memory:", 1, slog.New(slog.NewTextHandler(authTestLogWriter{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	return store
}

type authIDs struct{ next int }

func (g *authIDs) New(prefix string) string {
	g.next++
	return prefix + "-" + string(rune('a'+g.next))
}

type authTokens struct{ next int }

func (g *authTokens) NewToken() (string, error) {
	g.next++
	return "token-" + string(rune('a'+g.next)), nil
}

func seedAuthUser(t *testing.T, store *sqlite.Store, id string, role domain.Role, active bool) domain.User {
	t.Helper()
	hash, err := HashPassword("a-valid-password")
	if err != nil {
		t.Fatal(err)
	}
	now := store.Now()
	user := domain.User{ID: id, TenantID: "museum-demo", Email: id + "@museum.invalid", DisplayName: id,
		Role: role, PasswordHash: hash, Active: active, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

func newAuthService(store *sqlite.Store) *Service {
	return &Service{Users: store, Sessions: store, Deactivator: store, IDs: &authIDs{}, Tokens: &authTokens{},
		SessionTTL: 2 * time.Hour, Now: store.Now}
}

func TestPasswordHashPolicyAndTokenComparison(t *testing.T) {
	if _, err := HashPassword("short"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected short password rejection, got %v", err)
	}
	hash, err := HashPassword("a-valid-password")
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) == 0 || EqualTokenHash(HashToken("same"), HashToken("different")) {
		t.Fatal("password/token primitives returned invalid result")
	}
	if !EqualTokenHash(HashToken("same"), HashToken("same")) {
		t.Fatal("equal token hashes should compare equal")
	}
}

func TestLoginAuthenticateAndLogoutLifecycle(t *testing.T) {
	store := authStore(t)
	user := seedAuthUser(t, store, "supervisor", domain.RoleSupervisor, true)
	service := newAuthService(store)
	result, err := service.Login(context.Background(), LoginInput{TenantID: user.TenantID, Email: user.Email, Password: "a-valid-password"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.Principal.UserID != user.ID || result.ExpiresAt.IsZero() {
		t.Fatalf("invalid login result: %#v", result)
	}
	principal, err := service.Authenticate(context.Background(), result.Token)
	if err != nil || principal.SessionID == "" || principal.Role != domain.RoleSupervisor {
		t.Fatalf("authentication failed: %#v %v", principal, err)
	}
	if err := service.Logout(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), result.Token); !errors.Is(err, domain.ErrRevoked) {
		t.Fatalf("revoked session should be rejected, got %v", err)
	}
}

func TestLoginNormalizesCredentialFailures(t *testing.T) {
	store := authStore(t)
	user := seedAuthUser(t, store, "registrar", domain.RoleRegistrar, true)
	service := newAuthService(store)
	tests := []LoginInput{
		{TenantID: user.TenantID, Email: "missing@museum.invalid", Password: "a-valid-password"},
		{TenantID: user.TenantID, Email: user.Email, Password: "wrong-password"},
	}
	for _, input := range tests {
		if _, err := service.Login(context.Background(), input); !errors.Is(err, domain.ErrUnauthorized) {
			t.Errorf("expected unauthorized login, got %v", err)
		}
	}
	if _, err := service.Login(context.Background(), LoginInput{TenantID: user.TenantID, Email: user.Email}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected invalid empty credentials, got %v", err)
	}
	inactive := seedAuthUser(t, store, "inactive", domain.RoleRegistrar, false)
	if _, err := service.Login(context.Background(), LoginInput{TenantID: inactive.TenantID, Email: inactive.Email, Password: "a-valid-password"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected inactive user rejection, got %v", err)
	}
}

func TestCreateUserRequiresSupervisorAndHashesPassword(t *testing.T) {
	store := authStore(t)
	supervisor := seedAuthUser(t, store, "supervisor", domain.RoleSupervisor, true)
	registrar := seedAuthUser(t, store, "registrar", domain.RoleRegistrar, true)
	authService := newAuthService(store)
	ctx := service.WithPrincipal(context.Background(), domain.Principal{TenantID: supervisor.TenantID, UserID: supervisor.ID, Role: supervisor.Role})
	created, err := authService.CreateUser(ctx, CreateUserInput{Email: "new@museum.invalid", DisplayName: "New Registrar", Role: domain.RoleRegistrar, Password: "another-valid-password"})
	if err != nil {
		t.Fatal(err)
	}
	if created.PasswordHash == nil || string(created.PasswordHash) == "another-valid-password" {
		t.Fatal("created user password was not hashed")
	}
	if _, err := store.FindUser(context.Background(), created.TenantID, created.ID); err != nil {
		t.Fatal(err)
	}
	registrarCtx := service.WithPrincipal(context.Background(), domain.Principal{TenantID: registrar.TenantID, UserID: registrar.ID, Role: registrar.Role})
	if _, err := authService.CreateUser(registrarCtx, CreateUserInput{Email: "denied@museum.invalid", DisplayName: "Denied", Role: domain.RoleRegistrar, Password: "another-valid-password"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected non-supervisor create to fail, got %v", err)
	}
}

func TestDeactivateUserAtomicallyRevokesSessions(t *testing.T) {
	store := authStore(t)
	supervisor := seedAuthUser(t, store, "supervisor", domain.RoleSupervisor, true)
	target := seedAuthUser(t, store, "target", domain.RoleRegistrar, true)
	authService := newAuthService(store)
	login, err := authService.Login(context.Background(), LoginInput{TenantID: target.TenantID, Email: target.Email, Password: "a-valid-password"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := service.WithPrincipal(service.WithRequestID(context.Background(), "request-deactivate"), domain.Principal{TenantID: supervisor.TenantID, UserID: supervisor.ID, Role: supervisor.Role})
	if err := authService.DeactivateUser(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := authService.Authenticate(context.Background(), login.Token); !errors.Is(err, domain.ErrRevoked) {
		t.Fatalf("target session should be revoked, got %v", err)
	}
	loaded, err := store.FindUser(context.Background(), target.TenantID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Active {
		t.Fatal("target user remained active")
	}
	audit, err := store.List(context.Background(), supervisor.TenantID, target.ID, domain.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 || audit[0].Action != "user.deactivate" {
		t.Fatalf("deactivation audit missing: %#v", audit)
	}
}

func TestAuthenticationHonorsContextCancellation(t *testing.T) {
	store := authStore(t)
	user := seedAuthUser(t, store, "registrar", domain.RoleRegistrar, true)
	service := newAuthService(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Login(ctx, LoginInput{TenantID: user.TenantID, Email: user.Email, Password: "a-valid-password"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled login, got %v", err)
	}
	if _, err := service.Authenticate(ctx, "token"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled authentication, got %v", err)
	}
}

// cancellationSpySessions returns a valid session and then cancels the caller
// context, simulating a request that goes away after the session loaded but
// before the principal is resolved.
type cancellationSpySessions struct {
	store    repository.SessionRepository
	onLookup func(ctx context.Context)
	called   bool
}

func (s *cancellationSpySessions) CreateSession(ctx context.Context, session domain.Session) error {
	return s.store.CreateSession(ctx, session)
}

func (s *cancellationSpySessions) FindSessionByToken(ctx context.Context, tokenHash []byte) (domain.Session, error) {
	session, err := s.store.FindSessionByToken(ctx, tokenHash)
	if err == nil && !s.called {
		s.called = true
		s.onLookup(ctx)
	}
	return session, err
}

func (s *cancellationSpySessions) RevokeSession(ctx context.Context, tenantID, sessionID string, at time.Time) error {
	return s.store.RevokeSession(ctx, tenantID, sessionID, at)
}

func (s *cancellationSpySessions) RevokeUserSessions(ctx context.Context, tenantID, userID string, at time.Time) error {
	return s.store.RevokeUserSessions(ctx, tenantID, userID, at)
}

// userAccessSpy records whether the user store was reached after cancellation.
type userAccessSpy struct {
	repository.UserRepository
	accessed bool
}

func (u *userAccessSpy) FindUser(ctx context.Context, tenantID, id string) (domain.User, error) {
	u.accessed = true
	return u.UserRepository.FindUser(ctx, tenantID, id)
}

func TestAuthenticateStopsOnCancellationAfterSessionLoads(t *testing.T) {
	store := authStore(t)
	user := seedAuthUser(t, store, "registrar", domain.RoleRegistrar, true)
	authService := newAuthService(store)

	login, err := authService.Login(context.Background(), LoginInput{TenantID: user.TenantID, Email: user.Email, Password: "a-valid-password"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	spy := &userAccessSpy{UserRepository: store}
	authService.Users = spy
	authService.Sessions = &cancellationSpySessions{
		store:    store,
		onLookup: func(_ context.Context) { cancel() },
	}

	_, err = authService.Authenticate(ctx, login.Token)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled authentication after session load, got %v", err)
	}
	if spy.accessed {
		t.Fatal("user data must not be accessed once the request was cancelled")
	}
}
