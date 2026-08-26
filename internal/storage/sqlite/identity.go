package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (s *Store) CreateUser(ctx context.Context, user domain.User) error {
	if err := isCancelled(ctx); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO users(
			id, tenant_id, email, display_name, role, password_hash,
			active, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.TenantID, user.Email, user.DisplayName, user.Role, user.PasswordHash,
		boolInt(user.Active), user.Version, timeText(user.CreatedAt), timeText(user.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create user: %w", domain.ErrAlreadyExists)
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *Store) FindUserByEmail(ctx context.Context, tenantID, email string) (domain.User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, display_name, role, password_hash,
		       active, version, created_at, updated_at
		FROM users WHERE tenant_id = ? AND email = ?
	`, tenantID, email))
}

func (s *Store) FindUser(ctx context.Context, tenantID, id string) (domain.User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, display_name, role, password_hash,
		       active, version, created_at, updated_at
		FROM users WHERE tenant_id = ? AND id = ?
	`, tenantID, id))
}

func scanUser(row *sql.Row) (domain.User, error) {
	var user domain.User
	var active int
	var createdAt, updatedAt string
	err := row.Scan(&user.ID, &user.TenantID, &user.Email, &user.DisplayName, &user.Role,
		&user.PasswordHash, &active, &user.Version, &createdAt, &updatedAt)
	if err != nil {
		return domain.User{}, isNoRows(err)
	}
	user.Active = active != 0
	if user.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.User{}, fmt.Errorf("scan user created_at: %w", err)
	}
	if user.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.User{}, fmt.Errorf("scan user updated_at: %w", err)
	}
	user.PasswordHash = append([]byte(nil), user.PasswordHash...)
	return user, nil
}

func (s *Store) SetUserActive(ctx context.Context, tenantID, id string, active bool, expectedVersion int64) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE users
		SET active = ?, version = version + 1, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND version = ?
	`, boolInt(active), timeText(s.Now()), tenantID, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("set user active: %w", err)
	}
	return requireChanged(result, "user", domain.ErrVersion)
}

func (s *Store) ListActiveSessions(ctx context.Context, tenantID, userID string) ([]domain.Session, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, tenant_id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM sessions
		WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?
		ORDER BY created_at
	`, tenantID, userID, timeText(s.Now()))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer closeQuietly(rows)
	var sessions []domain.Session
	for rows.Next() {
		session, scanErr := scanSessionRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}

func (s *Store) CreateSession(ctx context.Context, session domain.Session) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sessions(id, tenant_id, user_id, token_hash, expires_at, revoked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.TenantID, session.UserID, session.TokenHash,
		timeText(session.ExpiresAt), nullableTime(session.RevokedAt), timeText(session.CreatedAt))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) FindSessionByToken(ctx context.Context, tokenHash []byte) (domain.Session, error) {
	if err := isCancelled(ctx); err != nil {
		return domain.Session{}, err
	}
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM sessions WHERE token_hash = ?
	`, tokenHash)
	session, err := scanSessionRow(row)
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func scanSessionRow(row *sql.Row) (domain.Session, error) {
	var session domain.Session
	var expiresAt, createdAt string
	var revokedAt sql.NullString
	err := row.Scan(&session.ID, &session.TenantID, &session.UserID, &session.TokenHash,
		&expiresAt, &revokedAt, &createdAt)
	if err != nil {
		return domain.Session{}, isNoRows(err)
	}
	return finishSession(session, expiresAt, revokedAt, createdAt)
}

func scanSessionRows(rows *sql.Rows) (domain.Session, error) {
	var session domain.Session
	var expiresAt, createdAt string
	var revokedAt sql.NullString
	err := rows.Scan(&session.ID, &session.TenantID, &session.UserID, &session.TokenHash,
		&expiresAt, &revokedAt, &createdAt)
	if err != nil {
		return domain.Session{}, fmt.Errorf("scan session: %w", err)
	}
	return finishSession(session, expiresAt, revokedAt, createdAt)
}

func finishSession(session domain.Session, expiresAt string, revokedAt sql.NullString, createdAt string) (domain.Session, error) {
	var err error
	if session.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return domain.Session{}, fmt.Errorf("scan session expires_at: %w", err)
	}
	if session.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Session{}, fmt.Errorf("scan session created_at: %w", err)
	}
	if revokedAt.Valid {
		value, parseErr := parseTime(revokedAt.String)
		if parseErr != nil {
			return domain.Session{}, fmt.Errorf("scan session revoked_at: %w", parseErr)
		}
		session.RevokedAt = &value
	}
	session.TokenHash = append([]byte(nil), session.TokenHash...)
	return session, nil
}

func (s *Store) RevokeSession(ctx context.Context, tenantID, sessionID string, at time.Time) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?
		WHERE tenant_id = ? AND id = ? AND revoked_at IS NULL
	`, timeText(at), tenantID, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return requireChanged(result, "session", domain.ErrNotFound)
}

func (s *Store) RevokeUserSessions(ctx context.Context, tenantID, userID string, at time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?
		WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL
	`, timeText(at), tenantID, userID)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func (s *Store) DeactivateUserAndSessions(ctx context.Context, actor domain.Principal, target domain.User, requestID string) error {
	result, err := s.DB.ExecContext(ctx, `
			UPDATE users SET active = 0, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ? AND active = 1
		`, timeText(s.Now()), target.TenantID, target.ID, target.Version)
	if err != nil {
		return fmt.Errorf("deactivate user: %w", err)
	}
	if err := requireChanged(result, "user", domain.ErrVersion); err != nil {
		return err
	}
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE sessions SET revoked_at = ?
			WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL
		`, timeText(s.Now()), target.TenantID, target.ID); err != nil {
			return fmt.Errorf("revoke sessions during deactivation: %w", err)
		}
		if err := insertAudit(ctx, tx, domain.AuditEvent{
			ID: newID("audit"), TenantID: target.TenantID, ActorID: actor.UserID,
			Action: "user.deactivate", ObjectType: "user", ObjectID: target.ID,
			Result: "success", RequestID: requestID, Details: []byte(`{"sessions":"revoked"}`), CreatedAt: s.Now(),
		}); err != nil {
			return err
		}
		return nil
	})
}

func requireChanged(result sql.Result, entity string, failure error) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s affected rows: %w", entity, err)
	}
	if count == 0 {
		return fmt.Errorf("%s: %w", entity, failure)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (errors.Is(err, domain.ErrAlreadyExists) || containsAny(err.Error(), "UNIQUE constraint failed", "constraint failed"))
}
