package idempotency

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type Store struct {
	DB  *sql.DB
	Now func() time.Time
}

type Scope struct {
	TenantID string
	Method   string
	Path     string
	Key      string
	BodyHash string
}

type Record struct {
	Scope
	Status     string
	HTTPStatus int
	Body       []byte
	ResourceID string
	ExpiresAt  time.Time
}

func NewScope(tenantID, method, path, key string, body []byte) (Scope, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	key = strings.TrimSpace(key)
	if tenantID == "" || method == "" || path == "" || key == "" {
		return Scope{}, domain.FieldError{Field: "idempotency", Message: "tenant, method, path and key are required"}
	}
	if len(key) > 128 {
		return Scope{}, domain.FieldError{Field: "idempotency_key", Message: "cannot exceed 128 bytes"}
	}
	digest := sha256.Sum256(body)
	return Scope{TenantID: tenantID, Method: method, Path: path, Key: key, BodyHash: hex.EncodeToString(digest[:])}, nil
}

func (s *Store) Begin(ctx context.Context, scope Scope, ttl time.Duration) (Record, bool, error) {
	if ttl <= 0 {
		return Record{}, false, domain.FieldError{Field: "ttl", Message: "must be positive"}
	}
	now := s.Now().UTC()
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM idempotency_keys
		WHERE tenant_id = ? AND method = ? AND path = ? AND request_key = ? AND expires_at <= ?
	`, scope.TenantID, scope.Method, scope.Path, scope.Key, now.Format(time.RFC3339Nano))
	if err != nil {
		return Record{}, false, fmt.Errorf("expire idempotency key: %w", err)
	}
	result, err := s.DB.ExecContext(ctx, `
		INSERT INTO idempotency_keys(
			tenant_id, method, path, request_key, request_hash,
			response_status, response_body, resource_id, state,
			expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, NULL, NULL, NULL, 'started', ?, ?, ?)
		ON CONFLICT(tenant_id, method, path, request_key) DO NOTHING
	`, scope.TenantID, scope.Method, scope.Path, scope.Key, scope.BodyHash,
		now.Add(ttl).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Record{}, false, fmt.Errorf("begin idempotent request: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Record{}, false, fmt.Errorf("check idempotent insert: %w", err)
	}
	if rows == 1 {
		return Record{Scope: scope, Status: "started", ExpiresAt: now.Add(ttl)}, true, nil
	}
	record, err := s.Get(ctx, scope)
	if err != nil {
		return Record{}, false, err
	}
	if record.BodyHash != scope.BodyHash {
		return Record{}, false, fmt.Errorf("idempotency key reused with different body: %w", domain.ErrConflict)
	}
	return record, false, nil
}

func (s *Store) Complete(ctx context.Context, scope Scope, status int, body []byte, resourceID string) error {
	if status < 100 || status > 599 {
		return domain.FieldError{Field: "response_status", Message: "invalid HTTP status"}
	}
	result, err := s.DB.ExecContext(ctx, `
		UPDATE idempotency_keys
		SET response_status = ?, response_body = ?, resource_id = ?, state = 'completed', updated_at = ?
		WHERE tenant_id = ? AND method = ? AND path = ? AND request_key = ?
		  AND request_hash = ? AND state = 'started' AND expires_at > ?
	`, status, append([]byte(nil), body...), resourceID, s.Now().UTC().Format(time.RFC3339Nano),
		scope.TenantID, scope.Method, scope.Path, scope.Key, scope.BodyHash, s.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("complete idempotent request: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("idempotency ownership lost: %w", domain.ErrConflict)
	}
	return nil
}

func (s *Store) Forget(ctx context.Context, scope Scope) error {
	query := `
		DELETE FROM idempotency_keys
		WHERE tenant_id = ? AND method = ? AND path = ? AND request_key = ?
		  AND request_hash = ?
	`
	_, err := s.DB.ExecContext(ctx, query, scope.TenantID, scope.Method, scope.Path, scope.Key, scope.BodyHash)
	if err != nil {
		return fmt.Errorf("forget idempotent request: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, scope Scope) (Record, error) {
	var record Record
	var responseStatus sql.NullInt64
	var responseBody []byte
	var resourceID sql.NullString
	var expiresAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT tenant_id, method, path, request_key, request_hash,
		       response_status, response_body, resource_id, state, expires_at
		FROM idempotency_keys
		WHERE tenant_id = ? AND method = ? AND path = ? AND request_key = ?
	`, scope.TenantID, scope.Method, scope.Path, scope.Key).Scan(
		&record.TenantID, &record.Method, &record.Path, &record.Key, &record.BodyHash,
		&responseStatus, &responseBody, &resourceID, &record.Status, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, domain.ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get idempotent request: %w", err)
	}
	record.HTTPStatus = int(responseStatus.Int64)
	record.Body = append([]byte(nil), responseBody...)
	record.ResourceID = resourceID.String
	record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return Record{}, fmt.Errorf("parse idempotency expiry: %w", err)
	}
	return record, nil
}

func (r Record) Replayable() bool {
	return r.Status == "completed" && r.HTTPStatus >= http.StatusContinue && r.HTTPStatus <= 599
}
