package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

const artifactColumns = `id, tenant_id, accession_no, name, material, period, risk_class,
status, current_zone_id, current_case_id, active_loan_id, last_report_id,
version, created_at, updated_at`

func (s *Store) CreateArtifact(ctx context.Context, artifact domain.Artifact, report domain.ConditionReport, custody domain.CustodyEvent, audit domain.AuditEvent) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := insertArtifact(ctx, tx, artifact); err != nil {
			return err
		}
		if err := insertConditionReport(ctx, tx, report); err != nil {
			return err
		}
		if err := insertCustody(ctx, tx, custody); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, audit); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE artifacts SET last_report_id = ?, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ?
		`, report.ID, timeText(s.Now()), artifact.TenantID, artifact.ID, artifact.Version)
		if err != nil {
			return fmt.Errorf("link initial condition report: %w", err)
		}
		if err := requireChanged(result, "artifact", domain.ErrVersion); err != nil {
			return err
		}
		return nil
	})
}

func insertArtifact(ctx context.Context, tx *sql.Tx, artifact domain.Artifact) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO artifacts(
			id, tenant_id, accession_no, name, material, period, risk_class,
			status, current_zone_id, current_case_id, active_loan_id, last_report_id,
			version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, artifact.ID, artifact.TenantID, artifact.AccessionNo, artifact.Name, artifact.Material,
		artifact.Period, artifact.RiskClass, artifact.Status, artifact.CurrentZoneID,
		nullableString(artifact.CurrentCaseID), nullableString(artifact.ActiveLoanID),
		nullableString(artifact.LastReportID), artifact.Version,
		timeText(artifact.CreatedAt), timeText(artifact.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create artifact: %w", domain.ErrAlreadyExists)
		}
		return fmt.Errorf("create artifact: %w", err)
	}
	return nil
}

func (s *Store) GetArtifact(ctx context.Context, tenantID, id string) (domain.Artifact, error) {
	return scanArtifact(s.DB.QueryRowContext(ctx, `SELECT `+artifactColumns+` FROM artifacts WHERE tenant_id = ? AND id = ?`, tenantID, id))
}

func scanArtifact(row *sql.Row) (domain.Artifact, error) {
	var artifact domain.Artifact
	var currentCase, activeLoan, lastReport sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&artifact.ID, &artifact.TenantID, &artifact.AccessionNo, &artifact.Name,
		&artifact.Material, &artifact.Period, &artifact.RiskClass, &artifact.Status,
		&artifact.CurrentZoneID, &currentCase, &activeLoan, &lastReport,
		&artifact.Version, &createdAt, &updatedAt)
	if err != nil {
		return domain.Artifact{}, isNoRows(err)
	}
	artifact.CurrentCaseID = nullString(currentCase)
	artifact.ActiveLoanID = nullString(activeLoan)
	artifact.LastReportID = nullString(lastReport)
	if artifact.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Artifact{}, fmt.Errorf("scan artifact created_at: %w", err)
	}
	if artifact.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Artifact{}, fmt.Errorf("scan artifact updated_at: %w", err)
	}
	return artifact, nil
}

func scanArtifactRows(rows *sql.Rows) (domain.Artifact, error) {
	var artifact domain.Artifact
	var currentCase, activeLoan, lastReport sql.NullString
	var createdAt, updatedAt string
	err := rows.Scan(&artifact.ID, &artifact.TenantID, &artifact.AccessionNo, &artifact.Name,
		&artifact.Material, &artifact.Period, &artifact.RiskClass, &artifact.Status,
		&artifact.CurrentZoneID, &currentCase, &activeLoan, &lastReport,
		&artifact.Version, &createdAt, &updatedAt)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("scan artifact: %w", err)
	}
	artifact.CurrentCaseID = nullString(currentCase)
	artifact.ActiveLoanID = nullString(activeLoan)
	artifact.LastReportID = nullString(lastReport)
	if artifact.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Artifact{}, err
	}
	if artifact.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Artifact{}, err
	}
	return artifact, nil
}

func (s *Store) UpdateArtifactStatus(ctx context.Context, tenantID, id string, status domain.ArtifactStatus, expectedVersion int64) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE artifacts SET status = ?, version = version + 1, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND version = ?
	`, status, timeText(s.Now()), tenantID, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("update artifact status: %w", err)
	}
	return requireChanged(result, "artifact", domain.ErrVersion)
}

func (s *Store) ListArtifacts(ctx context.Context, tenantID string, page domain.Page, status string) ([]domain.Artifact, int, error) {
	page = page.Normalize()
	where := "tenant_id = ?"
	args := []any{tenantID}
	if strings.TrimSpace(status) != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count artifacts: %w", err)
	}
	queryArgs := append(append([]any(nil), args...), page.Limit, page.Offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+artifactColumns+` FROM artifacts WHERE `+where+` ORDER BY updated_at DESC, id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list artifacts: %w", err)
	}
	defer closeQuietly(rows)
	artifacts := make([]domain.Artifact, 0, page.Limit)
	for rows.Next() {
		artifact, scanErr := scanArtifactRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate artifacts: %w", err)
	}
	return artifacts, total, nil
}

func (s *Store) SaveConditionReport(ctx context.Context, report domain.ConditionReport, artifactVersion int64) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := insertConditionReport(ctx, tx, report); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE artifacts
			SET last_report_id = ?, status = ?, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ?
		`, report.ID, domain.ArtifactAssessment, timeText(s.Now()), report.TenantID, report.ArtifactID, artifactVersion)
		if err != nil {
			return fmt.Errorf("link condition report: %w", err)
		}
		return requireChanged(result, "artifact", domain.ErrVersion)
	})
}

func insertConditionReport(ctx context.Context, tx *sql.Tx, report domain.ConditionReport) error {
	measurements, err := json.Marshal(report.Measurements)
	if err != nil {
		return fmt.Errorf("encode condition measurements: %w", err)
	}
	issues, err := json.Marshal(report.ObservedIssues)
	if err != nil {
		return fmt.Errorf("encode condition issues: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO condition_reports(
			id, tenant_id, artifact_id, inspector_id, summary, severity,
			measurements_json, observed_issues_json, final, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, report.ID, report.TenantID, report.ArtifactID, report.InspectorID,
		report.Summary, report.Severity, string(measurements), string(issues), boolInt(report.Final), timeText(report.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert condition report: %w", err)
	}
	return nil
}

func (s *Store) GetConditionReport(ctx context.Context, tenantID, id string) (domain.ConditionReport, error) {
	var report domain.ConditionReport
	var measurements, issues string
	var final int
	var createdAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, artifact_id, inspector_id, summary, severity,
		       measurements_json, observed_issues_json, final, created_at
		FROM condition_reports WHERE tenant_id = ? AND id = ?
	`, tenantID, id).Scan(&report.ID, &report.TenantID, &report.ArtifactID, &report.InspectorID,
		&report.Summary, &report.Severity, &measurements, &issues, &final, &createdAt)
	if err != nil {
		return domain.ConditionReport{}, isNoRows(err)
	}
	if err := json.Unmarshal([]byte(measurements), &report.Measurements); err != nil {
		return domain.ConditionReport{}, fmt.Errorf("decode condition measurements: %w", err)
	}
	if err := json.Unmarshal([]byte(issues), &report.ObservedIssues); err != nil {
		return domain.ConditionReport{}, fmt.Errorf("decode condition issues: %w", err)
	}
	report.Final = final != 0
	if report.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.ConditionReport{}, fmt.Errorf("scan condition report time: %w", err)
	}
	return report.Clone(), nil
}

func insertCustody(ctx context.Context, tx *sql.Tx, event domain.CustodyEvent) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO custody_events(
			id, tenant_id, artifact_id, loan_id, from_holder, to_holder,
			location, seal_number, kind, occurred_at, recorded_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.TenantID, event.ArtifactID, nullableString(event.LoanID),
		event.FromHolder, event.ToHolder, event.Location, event.SealNumber,
		event.Kind, timeText(event.OccurredAt), event.RecordedBy)
	if err != nil {
		return fmt.Errorf("insert custody event: %w", err)
	}
	return nil
}
