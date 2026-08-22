package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (s *Store) OpenQuarantine(ctx context.Context, quarantine domain.QuarantineCase, artifact domain.Artifact, expectedVersion int64) error {
	zoneResult, err := s.DB.ExecContext(ctx, `
			UPDATE quarantine_zones
			SET occupied = occupied + 1, version = version + 1
			WHERE tenant_id = ? AND id = ? AND active = 1 AND occupied < capacity
		`, quarantine.TenantID, quarantine.ZoneID)
	if err != nil {
		return fmt.Errorf("reserve quarantine zone: %w", err)
	}
	if err := requireChanged(zoneResult, "quarantine zone", domain.ErrCapacity); err != nil {
		return err
	}
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO quarantine_cases(
				id, tenant_id, artifact_id, zone_id, reason, status,
				opened_by, resolved_by, version, opened_at, resolved_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, NULL)
		`, quarantine.ID, quarantine.TenantID, quarantine.ArtifactID, quarantine.ZoneID,
			quarantine.Reason, quarantine.Status, quarantine.OpenedBy, quarantine.Version,
			timeText(quarantine.OpenedAt))
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("open quarantine: %w", domain.ErrAlreadyExists)
			}
			return fmt.Errorf("open quarantine: %w", err)
		}
		artifactResult, err := tx.ExecContext(ctx, `
			UPDATE artifacts
			SET status = ?, current_zone_id = ?, current_case_id = ?,
			    version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ? AND status IN (?, ?, ?, ?)
		`, domain.ArtifactQuarantined, quarantine.ZoneID, quarantine.ID,
			timeText(s.Now()), artifact.TenantID, artifact.ID, expectedVersion,
			domain.ArtifactRegistered, domain.ArtifactAssessment, domain.ArtifactReady, domain.ArtifactOnDisplay)
		if err != nil {
			return fmt.Errorf("move artifact to quarantine: %w", err)
		}
		if err := requireChanged(artifactResult, "artifact", domain.ErrVersion); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) GetQuarantine(ctx context.Context, tenantID, id string) (domain.QuarantineCase, error) {
	var item domain.QuarantineCase
	var resolvedBy, resolvedAt sql.NullString
	var openedAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, artifact_id, zone_id, reason, status, opened_by,
		       resolved_by, version, opened_at, resolved_at
		FROM quarantine_cases WHERE tenant_id = ? AND id = ?
	`, tenantID, id).Scan(&item.ID, &item.TenantID, &item.ArtifactID, &item.ZoneID,
		&item.Reason, &item.Status, &item.OpenedBy, &resolvedBy, &item.Version, &openedAt, &resolvedAt)
	if err != nil {
		return domain.QuarantineCase{}, isNoRows(err)
	}
	item.ResolvedBy = nullString(resolvedBy)
	if item.OpenedAt, err = parseTime(openedAt); err != nil {
		return domain.QuarantineCase{}, fmt.Errorf("scan quarantine opened_at: %w", err)
	}
	if resolvedAt.Valid {
		value, parseErr := parseTime(resolvedAt.String)
		if parseErr != nil {
			return domain.QuarantineCase{}, fmt.Errorf("scan quarantine resolved_at: %w", parseErr)
		}
		item.ResolvedAt = &value
	}
	return item, nil
}

func (s *Store) CreateTreatmentPlan(ctx context.Context, plan domain.TreatmentPlan) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var quarantineStatus domain.QuarantineStatus
		if err := tx.QueryRowContext(ctx, `
			SELECT status FROM quarantine_cases
			WHERE tenant_id = ? AND id = ? AND artifact_id = ?
		`, plan.TenantID, plan.QuarantineID, plan.ArtifactID).Scan(&quarantineStatus); err != nil {
			return isNoRows(err)
		}
		if quarantineStatus == domain.QuarantineResolved {
			return fmt.Errorf("create treatment plan: %w", domain.ErrPrecondition)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO treatment_plans(
				id, tenant_id, artifact_id, quarantine_id, conservator_id,
				procedure, evidence_uri, status, version, approved_by,
				created_at, updated_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, plan.ID, plan.TenantID, plan.ArtifactID, plan.QuarantineID, plan.ConservatorID,
			plan.Procedure, plan.EvidenceURI, plan.Status, plan.Version,
			nullableString(plan.ApprovedBy), timeText(plan.CreatedAt), timeText(plan.UpdatedAt), nullableTime(plan.CompletedAt))
		if err != nil {
			return fmt.Errorf("create treatment plan: %w", err)
		}
		return nil
	})
}

func (s *Store) GetTreatmentPlan(ctx context.Context, tenantID, id string) (domain.TreatmentPlan, error) {
	var plan domain.TreatmentPlan
	var approvedBy, completedAt sql.NullString
	var createdAt, updatedAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, artifact_id, quarantine_id, conservator_id,
		       procedure, evidence_uri, status, version, approved_by,
		       created_at, updated_at, completed_at
		FROM treatment_plans WHERE tenant_id = ? AND id = ?
	`, tenantID, id).Scan(&plan.ID, &plan.TenantID, &plan.ArtifactID, &plan.QuarantineID,
		&plan.ConservatorID, &plan.Procedure, &plan.EvidenceURI, &plan.Status, &plan.Version,
		&approvedBy, &createdAt, &updatedAt, &completedAt)
	if err != nil {
		return domain.TreatmentPlan{}, isNoRows(err)
	}
	plan.ApprovedBy = nullString(approvedBy)
	if plan.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.TreatmentPlan{}, err
	}
	if plan.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.TreatmentPlan{}, err
	}
	if completedAt.Valid {
		value, parseErr := parseTime(completedAt.String)
		if parseErr != nil {
			return domain.TreatmentPlan{}, parseErr
		}
		plan.CompletedAt = &value
	}
	return plan, nil
}

func (s *Store) UpdateTreatment(ctx context.Context, plan domain.TreatmentPlan, expectedVersion int64) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE treatment_plans
			SET status = ?, evidence_uri = ?, approved_by = ?, completed_at = ?,
			    version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ?
		`, plan.Status, plan.EvidenceURI, nullableString(plan.ApprovedBy), nullableTime(plan.CompletedAt),
			timeText(plan.UpdatedAt), plan.TenantID, plan.ID, expectedVersion)
		if err != nil {
			return fmt.Errorf("update treatment plan: %w", err)
		}
		if err := requireChanged(result, "treatment plan", domain.ErrVersion); err != nil {
			return err
		}
		if plan.Status == domain.TreatmentInProgress {
			if _, err := tx.ExecContext(ctx, `
				UPDATE quarantine_cases SET status = ?, version = version + 1
				WHERE tenant_id = ? AND id = ? AND status = ?
			`, domain.QuarantineTreating, plan.TenantID, plan.QuarantineID, domain.QuarantineOpen); err != nil {
				return fmt.Errorf("mark quarantine treating: %w", err)
			}
		}
		if plan.Status == domain.TreatmentCompleted {
			if stringsTrim(plan.EvidenceURI) == "" {
				return fmt.Errorf("complete treatment without evidence: %w", domain.ErrPrecondition)
			}
			if err := s.finishTreatmentTx(ctx, tx, plan); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) finishTreatmentTx(ctx context.Context, tx *sql.Tx, plan domain.TreatmentPlan) error {
	var zoneID string
	result, err := tx.ExecContext(ctx, `
		UPDATE quarantine_cases
		SET status = ?, resolved_by = ?, resolved_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status IN (?, ?)
	`, domain.QuarantineResolved, plan.ConservatorID, timeText(s.Now()), plan.TenantID,
		plan.QuarantineID, domain.QuarantineOpen, domain.QuarantineTreating)
	if err != nil {
		return fmt.Errorf("resolve quarantine: %w", err)
	}
	if err := requireChanged(result, "quarantine", domain.ErrPrecondition); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT zone_id FROM quarantine_cases WHERE id = ?`, plan.QuarantineID).Scan(&zoneID); err != nil {
		return fmt.Errorf("read quarantine zone: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE quarantine_zones
		SET occupied = occupied - 1, version = version + 1
		WHERE tenant_id = ? AND id = ? AND occupied > 0
	`, plan.TenantID, zoneID); err != nil {
		return fmt.Errorf("release quarantine zone: %w", err)
	}
	artifactResult, err := tx.ExecContext(ctx, `
		UPDATE artifacts
		SET status = ?, current_case_id = NULL, version = version + 1, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND current_case_id = ? AND status IN (?, ?)
	`, domain.ArtifactReady, timeText(s.Now()), plan.TenantID, plan.ArtifactID, plan.QuarantineID,
		domain.ArtifactQuarantined, domain.ArtifactTreatment)
	if err != nil {
		return fmt.Errorf("release treated artifact: %w", err)
	}
	return requireChanged(artifactResult, "artifact", domain.ErrPrecondition)
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
