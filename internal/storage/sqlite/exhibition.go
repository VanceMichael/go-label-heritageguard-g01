package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (s *Store) GetDisplayCase(ctx context.Context, tenantID, id string) (domain.DisplayCase, error) {
	var item domain.DisplayCase
	var artifactID, reservationTo sql.NullString
	var updatedAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, gallery, name, status, artifact_id,
		       min_humidity, max_humidity, min_temp_c, max_temp_c,
		       reservation_to, version, updated_at
		FROM display_cases WHERE tenant_id = ? AND id = ?
	`, tenantID, id).Scan(&item.ID, &item.TenantID, &item.Gallery, &item.Name,
		&item.Status, &artifactID, &item.MinHumidity, &item.MaxHumidity,
		&item.MinTempC, &item.MaxTempC, &reservationTo, &item.Version, &updatedAt)
	if err != nil {
		return domain.DisplayCase{}, isNoRows(err)
	}
	item.ArtifactID = nullString(artifactID)
	if reservationTo.Valid {
		value, parseErr := parseTime(reservationTo.String)
		if parseErr != nil {
			return domain.DisplayCase{}, fmt.Errorf("scan case reservation: %w", parseErr)
		}
		item.ReservationTo = &value
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.DisplayCase{}, fmt.Errorf("scan case updated_at: %w", err)
	}
	return item, nil
}

func (s *Store) ReserveDisplayCase(ctx context.Context, item domain.DisplayCase, expectedVersion int64) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE display_cases
		SET status = ?, artifact_id = ?, reservation_to = ?,
		    version = version + 1, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND version = ? AND status = ? AND artifact_id IS NULL
	`, domain.CaseReserved, item.ArtifactID, nullableTime(item.ReservationTo), timeText(s.Now()),
		item.TenantID, item.ID, expectedVersion, domain.CaseAvailable)
	if err != nil {
		return fmt.Errorf("reserve display case: %w", err)
	}
	return requireChanged(result, "display case", domain.ErrConflict)
}

func (s *Store) ActivateInstallation(ctx context.Context, installation domain.Installation, artifact domain.Artifact, displayCase domain.DisplayCase, artifactVersion, caseVersion int64) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if !installation.Complete() {
			return fmt.Errorf("activate installation: %w", domain.ErrPrecondition)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO installations(
				id, tenant_id, artifact_id, display_case_id, mount_verified,
				seal_verified, environment_ready, installed_by, installed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, installation.ID, installation.TenantID, installation.ArtifactID,
			installation.DisplayCaseID, boolInt(installation.MountVerified),
			boolInt(installation.SealVerified), boolInt(installation.EnvironmentReady),
			installation.InstalledBy, timeText(installation.InstalledAt)); err != nil {
			return fmt.Errorf("insert installation: %w", err)
		}
		caseResult, err := tx.ExecContext(ctx, `
			UPDATE display_cases
			SET status = ?, reservation_to = NULL, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND artifact_id = ? AND version = ? AND status = ?
		`, domain.CaseActive, timeText(s.Now()), displayCase.TenantID, displayCase.ID,
			artifact.ID, caseVersion, domain.CaseReserved)
		if err != nil {
			return fmt.Errorf("activate display case: %w", err)
		}
		if err := requireChanged(caseResult, "display case", domain.ErrVersion); err != nil {
			return err
		}
		artifactResult, err := tx.ExecContext(ctx, `
			UPDATE artifacts
			SET status = ?, current_case_id = ?, current_zone_id = ?,
			    version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ? AND status = ?
		`, domain.ArtifactOnDisplay, displayCase.ID, displayCase.Gallery, timeText(s.Now()),
			artifact.TenantID, artifact.ID, artifactVersion, domain.ArtifactReady)
		if err != nil {
			return fmt.Errorf("place artifact on display: %w", err)
		}
		return requireChanged(artifactResult, "artifact", domain.ErrVersion)
	})
}

func (s *Store) SaveReadingAndEnqueue(ctx context.Context, reading domain.EnvironmentReading, job domain.WorkerJob) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO environment_readings(
				id, tenant_id, display_case_id, device_id, sequence,
				temperature_c, humidity, observed_at, received_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, reading.ID, reading.TenantID, reading.DisplayCaseID, reading.DeviceID,
			reading.Sequence, reading.TemperatureC, reading.Humidity,
			timeText(reading.ObservedAt), timeText(reading.ReceivedAt))
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save reading: %w", domain.ErrAlreadyExists)
			}
			return fmt.Errorf("save reading: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO worker_jobs(
				id, tenant_id, kind, aggregate_id, payload, status, attempts,
				max_attempts, available_at, lease_owner, lease_expires_at,
				last_error, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, '', ?, ?)
		`, job.ID, job.TenantID, job.Kind, job.AggregateID, []byte(job.Payload),
			job.Status, job.Attempts, job.MaxAttempts, timeText(job.AvailableAt),
			timeText(job.CreatedAt), timeText(job.UpdatedAt))
		if err != nil {
			return fmt.Errorf("enqueue reading assessment: %w", err)
		}
		return nil
	})
}

func (s *Store) AssessEnvironment(ctx context.Context, tenantID, caseID string, start, end time.Time) (domain.EnvironmentAssessment, error) {
	if err := isCancelled(ctx); err != nil {
		return domain.EnvironmentAssessment{}, err
	}
	displayCase, err := s.GetDisplayCase(ctx, tenantID, caseID)
	if err != nil {
		return domain.EnvironmentAssessment{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT temperature_c, humidity
		FROM environment_readings
		WHERE tenant_id = ? AND display_case_id = ? AND observed_at >= ? AND observed_at <= ?
		ORDER BY observed_at, id
	`, tenantID, caseID, timeText(start), timeText(end))
	if err != nil {
		return domain.EnvironmentAssessment{}, fmt.Errorf("query environment readings: %w", err)
	}
	defer closeQuietly(rows)
	assessment := domain.EnvironmentAssessment{
		DisplayCaseID: caseID,
		Ready:         true,
		WindowStart:   start,
		WindowEnd:     end,
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return domain.EnvironmentAssessment{}, err
		}
		var temp, humidity float64
		if err := rows.Scan(&temp, &humidity); err != nil {
			return domain.EnvironmentAssessment{}, fmt.Errorf("scan environment reading: %w", err)
		}
		assessment.ReadingCount++
		if temp < displayCase.MinTempC || temp > displayCase.MaxTempC {
			assessment.Ready = false
			assessment.Reasons = append(assessment.Reasons, "temperature outside configured range")
		}
		if humidity < displayCase.MinHumidity || humidity > displayCase.MaxHumidity {
			assessment.Ready = false
			assessment.Reasons = append(assessment.Reasons, "humidity outside configured range")
		}
	}
	if err := rows.Err(); err != nil {
		return domain.EnvironmentAssessment{}, fmt.Errorf("iterate environment readings: %w", err)
	}
	if assessment.ReadingCount == 0 {
		assessment.Ready = false
		assessment.Reasons = append(assessment.Reasons, "no readings in assessment window")
	}
	assessment.Reasons = uniqueStrings(assessment.Reasons)
	return assessment, nil
}

func (s *Store) OpenIncidentAndOutbox(ctx context.Context, incident domain.Incident, event domain.OutboxEvent) (domain.Incident, bool, error) {
	var stored domain.Incident
	var created bool
	err := s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO incidents(
				id, tenant_id, display_case_id, artifact_id, window_key, kind,
				status, summary, remediated, version, opened_at, updated_at, closed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
			ON CONFLICT(tenant_id, display_case_id, window_key, kind) DO NOTHING
		`, incident.ID, incident.TenantID, incident.DisplayCaseID, nullableString(incident.ArtifactID),
			incident.WindowKey, incident.Kind, incident.Status, incident.Summary,
			boolInt(incident.Remediated), incident.Version, timeText(incident.OpenedAt), timeText(incident.UpdatedAt))
		if err != nil {
			return fmt.Errorf("open environment incident: %w", err)
		}
		rowsCreated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check environment incident insert: %w", err)
		}
		stored, err = scanIncident(tx.QueryRowContext(ctx, `
			SELECT id, tenant_id, display_case_id, artifact_id, window_key, kind,
			       status, summary, remediated, version, opened_at, updated_at, closed_at
			FROM incidents WHERE tenant_id = ? AND display_case_id = ? AND window_key = ? AND kind = ?
		`, incident.TenantID, incident.DisplayCaseID, incident.WindowKey, incident.Kind))
		if err != nil {
			return err
		}
		if rowsCreated == 0 {
			return nil
		}
		created = true
		event.AggregateID = stored.ID
		_, err = tx.ExecContext(ctx, `
			INSERT INTO outbox_events(
				id, tenant_id, topic, aggregate_id, idempotency_key, payload,
				status, attempts, max_attempts, available_at, lease_owner,
				lease_expires_at, last_error, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, '', ?, ?)
		`, event.ID, event.TenantID, event.Topic, event.AggregateID, event.IdempotencyKey,
			[]byte(event.Payload), event.Status, event.Attempts, event.MaxAttempts,
			timeText(event.AvailableAt), timeText(event.CreatedAt), timeText(event.UpdatedAt))
		if err != nil {
			return fmt.Errorf("enqueue incident outbox event: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Incident{}, false, err
	}
	return stored, created, nil
}

func (s *Store) GetIncident(ctx context.Context, tenantID, id string) (domain.Incident, error) {
	return scanIncident(s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, display_case_id, artifact_id, window_key, kind,
		       status, summary, remediated, version, opened_at, updated_at, closed_at
		FROM incidents WHERE tenant_id = ? AND id = ?
	`, tenantID, id))
}

func scanIncident(row *sql.Row) (domain.Incident, error) {
	var incident domain.Incident
	var artifactID, closedAt sql.NullString
	var remediated int
	var openedAt, updatedAt string
	err := row.Scan(&incident.ID, &incident.TenantID, &incident.DisplayCaseID,
		&artifactID, &incident.WindowKey, &incident.Kind, &incident.Status,
		&incident.Summary, &remediated, &incident.Version, &openedAt, &updatedAt, &closedAt)
	if err != nil {
		return domain.Incident{}, isNoRows(err)
	}
	incident.ArtifactID = nullString(artifactID)
	incident.Remediated = remediated != 0
	if incident.OpenedAt, err = parseTime(openedAt); err != nil {
		return domain.Incident{}, err
	}
	if incident.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Incident{}, err
	}
	if closedAt.Valid {
		value, parseErr := parseTime(closedAt.String)
		if parseErr != nil {
			return domain.Incident{}, parseErr
		}
		incident.ClosedAt = &value
	}
	return incident, nil
}

func (s *Store) UpdateIncident(ctx context.Context, incident domain.Incident, expectedVersion int64) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE incidents
			SET status = ?, summary = ?, remediated = ?, closed_at = ?,
			    version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ?
		`, incident.Status, incident.Summary, boolInt(incident.Remediated),
			nullableTime(incident.ClosedAt), timeText(incident.UpdatedAt),
			incident.TenantID, incident.ID, expectedVersion)
		if err != nil {
			return fmt.Errorf("update incident: %w", err)
		}
		if err := requireChanged(result, "incident", domain.ErrVersion); err != nil {
			return err
		}
		caseStatus := domain.CaseIncident
		if incident.Status == domain.IncidentClosed {
			caseStatus = domain.CaseActive
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE display_cases SET status = ?, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND status IN (?, ?)
		`, caseStatus, timeText(s.Now()), incident.TenantID, incident.DisplayCaseID,
			domain.CaseActive, domain.CaseIncident); err != nil {
			return fmt.Errorf("update incident display case: %w", err)
		}
		return nil
	})
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
