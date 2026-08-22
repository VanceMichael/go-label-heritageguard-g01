package domain

import "time"

type Role string

const (
	RoleRegistrar   Role = "registrar"
	RoleConservator Role = "conservator"
	RoleCoordinator Role = "coordinator"
	RoleSupervisor  Role = "supervisor"
)

func (r Role) Valid() bool {
	switch r {
	case RoleRegistrar, RoleConservator, RoleCoordinator, RoleSupervisor:
		return true
	default:
		return false
	}
}

type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	PasswordHash []byte    `json:"-"`
	Active       bool      `json:"active"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	UserID    string     `json:"user_id"`
	TokenHash []byte     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type Principal struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	Role      Role   `json:"role"`
}

func (p Principal) Can(roles ...Role) bool {
	for _, role := range roles {
		if p.Role == role {
			return true
		}
	}
	return false
}
