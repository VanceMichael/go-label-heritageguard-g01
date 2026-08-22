package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type AtomicDeactivator interface {
	DeactivateUserAndSessions(context.Context, domain.Principal, domain.User, string) error
}

type Service struct {
	Users       repository.UserRepository
	Sessions    repository.SessionRepository
	Deactivator AtomicDeactivator
	IDs         service.IDGenerator
	Tokens      service.TokenGenerator
	SessionTTL  time.Duration
	Now         func() time.Time
}

type LoginInput struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResult struct {
	Token     string           `json:"token"`
	ExpiresAt time.Time        `json:"expires_at"`
	Principal domain.Principal `json:"principal"`
}

type CreateUserInput struct {
	Email       string      `json:"email"`
	DisplayName string      `json:"display_name"`
	Role        domain.Role `json:"role"`
	Password    string      `json:"password"`
}

func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error) {
	principal, err := service.RequireRole(ctx, domain.RoleSupervisor)
	if err != nil {
		return domain.User{}, err
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.Email == "" || input.DisplayName == "" {
		return domain.User{}, domain.FieldError{Field: "user", Message: "email and display name are required"}
	}
	if !input.Role.Valid() {
		return domain.User{}, domain.FieldError{Field: "role", Message: "unsupported role"}
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}
	now := s.Now().UTC()
	user := domain.User{
		ID:           s.IDs.New("user"),
		TenantID:     principal.TenantID,
		Email:        input.Email,
		DisplayName:  input.DisplayName,
		Role:         input.Role,
		PasswordHash: passwordHash,
		Active:       true,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.Users.CreateUser(ctx, user); err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	if err := ctx.Err(); err != nil {
		return LoginResult{}, err
	}
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if input.TenantID == "" || input.Email == "" || input.Password == "" {
		return LoginResult{}, domain.FieldError{Field: "credentials", Message: "tenant, email and password are required"}
	}
	user, err := s.Users.FindUserByEmail(ctx, input.TenantID, input.Email)
	if err != nil {
		return LoginResult{}, fmt.Errorf("load login user: %w", normalizeCredentialError(err))
	}
	if !user.Active {
		return LoginResult{}, fmt.Errorf("user inactive: %w", domain.ErrForbidden)
	}
	if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(input.Password)); err != nil {
		return LoginResult{}, fmt.Errorf("password mismatch: %w", domain.ErrUnauthorized)
	}
	token, err := s.Tokens.NewToken()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.Now().UTC()
	expiresAt := now.Add(s.SessionTTL)
	session := domain.Session{
		ID:        s.IDs.New("session"),
		TenantID:  user.TenantID,
		UserID:    user.ID,
		TokenHash: HashToken(token),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	if err := s.Sessions.CreateSession(ctx, session); err != nil {
		return LoginResult{}, fmt.Errorf("persist login session: %w", err)
	}
	return LoginResult{
		Token:     token,
		ExpiresAt: expiresAt,
		Principal: domain.Principal{TenantID: user.TenantID, UserID: user.ID, SessionID: session.ID, Role: user.Role},
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (domain.Principal, error) {
	if err := ctx.Err(); err != nil {
		return domain.Principal{}, err
	}
	if strings.TrimSpace(token) == "" {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	session, err := s.Sessions.FindSessionByToken(ctx, HashToken(token))
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find session: %w", normalizeCredentialError(err))
	}
	if session.RevokedAt != nil {
		return domain.Principal{}, domain.ErrRevoked
	}
	if !s.Now().Before(session.ExpiresAt) {
		return domain.Principal{}, domain.ErrExpired
	}
	if err := ctx.Err(); err != nil {
		return domain.Principal{}, err
	}
	user, err := s.Users.FindUser(ctx, session.TenantID, session.UserID)
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find session user: %w", err)
	}
	if !user.Active {
		return domain.Principal{}, domain.ErrForbidden
	}
	return domain.Principal{TenantID: user.TenantID, UserID: user.ID, SessionID: session.ID, Role: user.Role}, nil
}

func (s *Service) Logout(ctx context.Context, principal domain.Principal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if principal.SessionID == "" {
		return domain.ErrUnauthorized
	}
	if err := s.Sessions.RevokeSession(ctx, principal.TenantID, principal.SessionID, s.Now()); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

func (s *Service) DeactivateUser(ctx context.Context, targetID string) error {
	actor, err := service.RequireRole(ctx, domain.RoleSupervisor)
	if err != nil {
		return err
	}
	if targetID == actor.UserID {
		return domain.FieldError{Field: "user_id", Message: "supervisor cannot deactivate the active account"}
	}
	target, err := s.Users.FindUser(ctx, actor.TenantID, targetID)
	if err != nil {
		return fmt.Errorf("find target user: %w", err)
	}
	if !target.Active {
		return fmt.Errorf("target user already inactive: %w", domain.ErrConflict)
	}
	if err := s.Deactivator.DeactivateUserAndSessions(ctx, actor, target, service.RequestIDFrom(ctx)); err != nil {
		return fmt.Errorf("deactivate user atomically: %w", err)
	}
	return nil
}

func HashPassword(password string) ([]byte, error) {
	if len(password) < 12 {
		return nil, domain.FieldError{Field: "password", Message: "must contain at least 12 characters"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

func HashToken(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	result := make([]byte, len(digest))
	copy(result, digest[:])
	return result
}

func EqualTokenHash(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare(left, right) == 1
}

func normalizeCredentialError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrUnauthorized
	}
	return err
}
