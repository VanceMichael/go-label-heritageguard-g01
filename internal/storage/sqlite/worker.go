package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (s *Store) EnqueueJob(ctx context.Context, job domain.WorkerJob) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO worker_jobs(
			id, tenant_id, kind, aggregate_id, payload, status, attempts,
			max_attempts, available_at, lease_owner, lease_expires_at,
			last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, '', ?, ?)
		ON CONFLICT(tenant_id, kind, aggregate_id) DO NOTHING
	`, job.ID, job.TenantID, job.Kind, job.AggregateID, []byte(job.Payload),
		job.Status, job.Attempts, job.MaxAttempts, timeText(job.AvailableAt),
		timeText(job.CreatedAt), timeText(job.UpdatedAt))
	if err != nil {
		return fmt.Errorf("enqueue worker job: %w", err)
	}
	return nil
}

func (s *Store) ClaimJob(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration) (domain.WorkerJob, error) {
	var claimed domain.WorkerJob
	err := s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var jobID string
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM worker_jobs
			WHERE status IN (?, ?, ?)
			  AND available_at <= ?
			ORDER BY available_at, created_at, id
			LIMIT 1
		`, domain.JobPending, domain.JobRetry, domain.JobRunning, timeText(now)).Scan(&jobID)
		if err != nil {
			return isNoRows(err)
		}
		leaseExpires := now.Add(leaseDuration)
		result, err := tx.ExecContext(ctx, `
			UPDATE worker_jobs
			SET status = ?, lease_owner = ?, lease_expires_at = ?,
			    attempts = attempts + 1, updated_at = ?
			WHERE id = ? AND status IN (?, ?)
			  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
		`, domain.JobRunning, owner, timeText(leaseExpires), timeText(now), jobID,
			domain.JobPending, domain.JobRetry, timeText(now))
		if err != nil {
			return fmt.Errorf("claim worker job: %w", err)
		}
		if err := requireChanged(result, "worker job", domain.ErrConflict); err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, `
			SELECT id, tenant_id, kind, aggregate_id, payload, status, attempts,
			       max_attempts, available_at, lease_owner, lease_expires_at,
			       last_error, created_at, updated_at
			FROM worker_jobs WHERE id = ?
		`, jobID)
		var scanErr error
		claimed, scanErr = scanJob(row)
		return scanErr
	})
	if err != nil {
		return domain.WorkerJob{}, err
	}
	return claimed, nil
}

func scanJob(row *sql.Row) (domain.WorkerJob, error) {
	var job domain.WorkerJob
	var payload []byte
	var leaseOwner, leaseExpires sql.NullString
	var availableAt, createdAt, updatedAt string
	err := row.Scan(&job.ID, &job.TenantID, &job.Kind, &job.AggregateID, &payload,
		&job.Status, &job.Attempts, &job.MaxAttempts, &availableAt, &leaseOwner,
		&leaseExpires, &job.LastError, &createdAt, &updatedAt)
	if err != nil {
		return domain.WorkerJob{}, isNoRows(err)
	}
	job.Payload = append([]byte(nil), payload...)
	job.LeaseOwner = nullString(leaseOwner)
	var parseErr error
	if job.AvailableAt, parseErr = parseTime(availableAt); parseErr != nil {
		return domain.WorkerJob{}, parseErr
	}
	if job.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return domain.WorkerJob{}, parseErr
	}
	if job.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return domain.WorkerJob{}, parseErr
	}
	if leaseExpires.Valid {
		value, err := parseTime(leaseExpires.String)
		if err != nil {
			return domain.WorkerJob{}, err
		}
		job.LeaseExpiresAt = &value
	}
	return job, nil
}

func (s *Store) CompleteJob(ctx context.Context, id, owner string, now time.Time) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE worker_jobs
		SET status = ?, lease_owner = NULL, lease_expires_at = NULL,
		    last_error = '', updated_at = ?
		WHERE id = ? AND status = ? AND lease_owner = ? AND lease_expires_at > ?
	`, domain.JobSucceeded, timeText(now), id, domain.JobRunning, owner, timeText(now))
	if err != nil {
		return fmt.Errorf("complete worker job: %w", err)
	}
	return requireChanged(result, "worker lease", domain.ErrLeaseLost)
}

func (s *Store) RetryJob(ctx context.Context, id, owner string, now, availableAt time.Time, cause error) error {
	message := errorText(cause)
	result, err := s.DB.ExecContext(ctx, `
		UPDATE worker_jobs
		SET status = ?, available_at = ?, lease_owner = NULL, lease_expires_at = NULL,
		    last_error = ?, updated_at = ?
		WHERE id = ? AND status = ? AND lease_owner = ? AND lease_expires_at > ?
	`, domain.JobRetry, timeText(availableAt), message, timeText(now), id,
		domain.JobRunning, owner, timeText(now))
	if err != nil {
		return fmt.Errorf("retry worker job: %w", err)
	}
	return requireChanged(result, "worker lease", domain.ErrLeaseLost)
}

func (s *Store) FailJob(ctx context.Context, id, owner string, now time.Time, cause error) error {
	message := errorText(cause)
	result, err := s.DB.ExecContext(ctx, `
		UPDATE worker_jobs
		SET status = ?, lease_owner = NULL, lease_expires_at = NULL,
		    last_error = ?, updated_at = ?
		WHERE id = ? AND status = ? AND lease_owner = ? AND lease_expires_at > ?
	`, domain.JobFailed, message, timeText(now), id, domain.JobRunning, owner, timeText(now))
	if err != nil {
		return fmt.Errorf("fail worker job: %w", err)
	}
	return requireChanged(result, "worker lease", domain.ErrLeaseLost)
}

func (s *Store) ReleaseLease(ctx context.Context, id, owner string, now time.Time) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE worker_jobs
		SET status = ?, available_at = ?, lease_owner = NULL, lease_expires_at = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND lease_owner = ?
	`, domain.JobRetry, timeText(now), timeText(now), id, domain.JobRunning, owner)
	if err != nil {
		return fmt.Errorf("release worker lease: %w", err)
	}
	return requireChanged(result, "worker lease", domain.ErrLeaseLost)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}
