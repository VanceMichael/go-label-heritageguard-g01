package service

import (
	"context"
	"fmt"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type principalKey struct{}
type requestIDKey struct{}

func WithPrincipal(ctx context.Context, principal domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFrom(ctx context.Context) (domain.Principal, error) {
	principal, ok := ctx.Value(principalKey{}).(domain.Principal)
	if !ok || principal.UserID == "" || principal.TenantID == "" {
		return domain.Principal{}, fmt.Errorf("principal missing: %w", domain.ErrUnauthorized)
	}
	return principal, nil
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestIDFrom(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func RequireRole(ctx context.Context, roles ...domain.Role) (domain.Principal, error) {
	principal, err := PrincipalFrom(ctx)
	if err != nil {
		return domain.Principal{}, err
	}
	if !principal.Can(roles...) {
		return domain.Principal{}, fmt.Errorf("role %s cannot perform operation: %w", principal.Role, domain.ErrForbidden)
	}
	return principal, nil
}
