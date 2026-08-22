package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/auth"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

type SupervisorConfig struct {
	TenantID string
	Email    string
	Name     string
	Password string
}

func EnsureSupervisor(
	ctx context.Context,
	users repository.UserRepository,
	ids service.IDGenerator,
	now func() time.Time,
	cfg SupervisorConfig,
) (bool, error) {
	cfg.Email = strings.ToLower(strings.TrimSpace(cfg.Email))
	cfg.Password = strings.TrimSpace(cfg.Password)
	if cfg.Email == "" && cfg.Password == "" {
		return false, nil
	}
	if cfg.Email == "" || cfg.Password == "" || strings.TrimSpace(cfg.TenantID) == "" {
		return false, domain.FieldError{Field: "bootstrap", Message: "tenant, email and password are required"}
	}
	existing, err := users.FindUserByEmail(ctx, cfg.TenantID, cfg.Email)
	if err == nil {
		if !existing.Active || existing.Role != domain.RoleSupervisor {
			return false, fmt.Errorf("bootstrap identity exists but is not an active supervisor: %w", domain.ErrConflict)
		}
		return false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return false, fmt.Errorf("check bootstrap supervisor: %w", err)
	}
	hash, err := auth.HashPassword(cfg.Password)
	if err != nil {
		return false, err
	}
	createdAt := now().UTC()
	user := domain.User{
		ID:           ids.New("user"),
		TenantID:     cfg.TenantID,
		Email:        cfg.Email,
		DisplayName:  strings.TrimSpace(cfg.Name),
		Role:         domain.RoleSupervisor,
		PasswordHash: hash,
		Active:       true,
		Version:      1,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
	if user.DisplayName == "" {
		return false, domain.FieldError{Field: "bootstrap_name", Message: "required"}
	}
	createCtx := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		createCtx, cancel = context.WithDeadline(createCtx, deadline)
		defer cancel()
	}
	if err := users.CreateUser(createCtx, user); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return false, nil
		}
		return false, fmt.Errorf("create bootstrap supervisor: %w", err)
	}
	return true, nil
}
