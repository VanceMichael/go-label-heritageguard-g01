package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (s *Store) EnqueueOutbox(ctx context.Context, event domain.OutboxEvent) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO outbox_events(
			id, tenant_id, topic, aggregate_id, idempotency_key, payload,
			status, attempts, max_attempts, available_at, lease_owner,
			lease_expires_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, '', ?, ?)
		ON CONFLICT(tenant_id, topic, idempotency_key) DO NOTHING
	`, event.ID, event.TenantID, event.Topic, event.AggregateID, event.IdempotencyKey,
		[]byte(event.Payload), event.Status, event.Attempts, event.MaxAttempts,
		timeText(event.AvailableAt), timeText(event.CreatedAt), timeText(event.UpdatedAt))
	if err != nil {
		return fmt.Errorf("enqueue outbox event: %w", err)
	}
	return nil
}

func (s *Store) ClaimOutbox(ctx context.Context, owner string, now time.Time, lease time.Duration) (domain.OutboxEvent, error) {
	var event domain.OutboxEvent
	err := s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM outbox_events
			WHERE status IN (?, ?) AND available_at <= ?
			  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
			ORDER BY available_at, created_at, id LIMIT 1
		`, domain.JobPending, domain.JobRetry, timeText(now), timeText(now)).Scan(&id); err != nil {
			return isNoRows(err)
		}
		expires := now.Add(lease)
		result, err := tx.ExecContext(ctx, `
			UPDATE outbox_events
			SET status = ?, attempts = attempts + 1, lease_owner = ?,
			    lease_expires_at = ?, updated_at = ?
			WHERE id = ? AND status IN (?, ?)
			  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
		`, domain.JobRunning, owner, timeText(expires), timeText(now), id,
			domain.JobPending, domain.JobRetry, timeText(now))
		if err != nil {
			return fmt.Errorf("claim outbox event: %w", err)
		}
		if err := requireChanged(result, "outbox lease", domain.ErrConflict); err != nil {
			return err
		}
		var scanErr error
		event, scanErr = scanOutbox(tx.QueryRowContext(ctx, `
			SELECT id, tenant_id, topic, aggregate_id, idempotency_key, payload,
			       status, attempts, max_attempts, available_at, lease_owner,
			       lease_expires_at, last_error, created_at, updated_at
			FROM outbox_events WHERE id = ?
		`, id))
		return scanErr
	})
	return event, err
}

func scanOutbox(row *sql.Row) (domain.OutboxEvent, error) {
	var event domain.OutboxEvent
	var payload []byte
	var leaseOwner, leaseExpires sql.NullString
	var availableAt, createdAt, updatedAt string
	err := row.Scan(&event.ID, &event.TenantID, &event.Topic, &event.AggregateID,
		&event.IdempotencyKey, &payload, &event.Status, &event.Attempts,
		&event.MaxAttempts, &availableAt, &leaseOwner, &leaseExpires,
		&event.LastError, &createdAt, &updatedAt)
	if err != nil {
		return domain.OutboxEvent{}, isNoRows(err)
	}
	event.Payload = append([]byte(nil), payload...)
	event.LeaseOwner = nullString(leaseOwner)
	var parseErr error
	if event.AvailableAt, parseErr = parseTime(availableAt); parseErr != nil {
		return domain.OutboxEvent{}, parseErr
	}
	if event.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return domain.OutboxEvent{}, parseErr
	}
	if event.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return domain.OutboxEvent{}, parseErr
	}
	if leaseExpires.Valid {
		value, err := parseTime(leaseExpires.String)
		if err != nil {
			return domain.OutboxEvent{}, err
		}
		event.LeaseExpiresAt = &value
	}
	return event, nil
}

func (s *Store) FinishOutbox(ctx context.Context, id, owner string, now time.Time, deliveryErr error) error {
	if deliveryErr == nil {
		result, err := s.DB.ExecContext(ctx, `
			UPDATE outbox_events SET status = ?, lease_owner = NULL, lease_expires_at = NULL,
			    last_error = '', updated_at = ?
			WHERE id = ? AND status = ? AND lease_owner = ? AND lease_expires_at > ?
		`, domain.JobSucceeded, timeText(now), id, domain.JobRunning, owner, timeText(now))
		if err != nil {
			return fmt.Errorf("complete outbox event: %w", err)
		}
		return requireChanged(result, "outbox lease", domain.ErrLeaseLost)
	}
	var attempts, maxAttempts int
	if err := s.DB.QueryRowContext(ctx, `SELECT attempts, max_attempts FROM outbox_events WHERE id = ?`, id).Scan(&attempts, &maxAttempts); err != nil {
		return isNoRows(err)
	}
	status := domain.JobRetry
	availableAt := now.Add(backoff(attempts))
	if attempts > maxAttempts {
		status = domain.JobFailed
		availableAt = now
	}
	result, err := s.DB.ExecContext(ctx, `
		UPDATE outbox_events
		SET status = ?, available_at = ?, lease_owner = NULL, lease_expires_at = NULL,
		    last_error = ?, updated_at = ?
		WHERE id = ? AND status = ? AND lease_owner = ? AND lease_expires_at > ?
	`, status, timeText(availableAt), errorText(deliveryErr), timeText(now), id,
		domain.JobRunning, owner, timeText(now))
	if err != nil {
		return fmt.Errorf("finish failed outbox event: %w", err)
	}
	return requireChanged(result, "outbox lease", domain.ErrLeaseLost)
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}
