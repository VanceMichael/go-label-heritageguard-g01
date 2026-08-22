package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

const loanColumns = `id, tenant_id, artifact_id, borrower, purpose, start_at, end_at,
status, courier_reference, version, created_by, approved_by, created_at, updated_at`

func (s *Store) CreateLoan(ctx context.Context, loan domain.LoanRequest) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO loan_requests(
			id, tenant_id, artifact_id, borrower, purpose, start_at, end_at,
			status, courier_reference, version, created_by, approved_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, loan.ID, loan.TenantID, loan.ArtifactID, loan.Borrower, loan.Purpose,
		timeText(loan.StartAt), timeText(loan.EndAt), loan.Status, loan.CourierReference,
		loan.Version, loan.CreatedBy, nullableString(loan.ApprovedBy), timeText(loan.CreatedAt), timeText(loan.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create loan: %w", err)
	}
	return nil
}

func (s *Store) GetLoan(ctx context.Context, tenantID, id string) (domain.LoanRequest, error) {
	return scanLoan(s.DB.QueryRowContext(ctx, `SELECT `+loanColumns+` FROM loan_requests WHERE tenant_id = ? AND id = ?`, tenantID, id))
}

func scanLoan(row *sql.Row) (domain.LoanRequest, error) {
	var loan domain.LoanRequest
	var approvedBy sql.NullString
	var startAt, endAt, createdAt, updatedAt string
	err := row.Scan(&loan.ID, &loan.TenantID, &loan.ArtifactID, &loan.Borrower,
		&loan.Purpose, &startAt, &endAt, &loan.Status, &loan.CourierReference,
		&loan.Version, &loan.CreatedBy, &approvedBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.LoanRequest{}, isNoRows(err)
	}
	loan.ApprovedBy = nullString(approvedBy)
	var parseErr error
	if loan.StartAt, parseErr = parseTime(startAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	if loan.EndAt, parseErr = parseTime(endAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	if loan.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	if loan.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	return loan, nil
}

func scanLoanRows(rows *sql.Rows) (domain.LoanRequest, error) {
	var loan domain.LoanRequest
	var approvedBy sql.NullString
	var startAt, endAt, createdAt, updatedAt string
	err := rows.Scan(&loan.ID, &loan.TenantID, &loan.ArtifactID, &loan.Borrower,
		&loan.Purpose, &startAt, &endAt, &loan.Status, &loan.CourierReference,
		&loan.Version, &loan.CreatedBy, &approvedBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.LoanRequest{}, fmt.Errorf("scan loan: %w", err)
	}
	loan.ApprovedBy = nullString(approvedBy)
	var parseErr error
	if loan.StartAt, parseErr = parseTime(startAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	if loan.EndAt, parseErr = parseTime(endAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	if loan.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	if loan.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return domain.LoanRequest{}, parseErr
	}
	return loan, nil
}

func (s *Store) UpdateLoan(ctx context.Context, loan domain.LoanRequest, expectedVersion int64) error {
	result, err := s.DB.ExecContext(ctx, `
		UPDATE loan_requests
		SET status = ?, courier_reference = ?, approved_by = ?,
		    version = version + 1, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND version = ?
	`, loan.Status, loan.CourierReference, nullableString(loan.ApprovedBy),
		timeText(loan.UpdatedAt), loan.TenantID, loan.ID, expectedVersion)
	if err != nil {
		return fmt.Errorf("update loan: %w", err)
	}
	return requireChanged(result, "loan", domain.ErrVersion)
}

func (s *Store) ListOverlappingLoans(ctx context.Context, tenantID, artifactID string, start, end time.Time) ([]domain.LoanRequest, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+loanColumns+`
		FROM loan_requests
		WHERE tenant_id = ? AND artifact_id = ?
		  AND status IN (?, ?, ?, ?)
		  AND start_at < ? AND ? < end_at
		ORDER BY start_at, id
	`, tenantID, artifactID, domain.LoanSubmitted, domain.LoanApproved,
		domain.LoanPacked, domain.LoanDispatched,
		timeText(end), timeText(start))
	if err != nil {
		return nil, fmt.Errorf("list overlapping loans: %w", err)
	}
	defer closeQuietly(rows)
	var loans []domain.LoanRequest
	for rows.Next() {
		loan, scanErr := scanLoanRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		loans = append(loans, loan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate overlapping loans: %w", err)
	}
	return loans, nil
}

func (s *Store) HasActiveArtifactIncidents(ctx context.Context, tenantID, artifactID string) (bool, error) {
	var count int
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM incidents
		WHERE tenant_id = ? AND artifact_id = ? AND status != ?
	`, tenantID, artifactID, domain.IncidentClosed).Scan(&count); err != nil {
		return false, fmt.Errorf("count active artifact incidents: %w", err)
	}
	return count > 0, nil
}

func (s *Store) RecordCustody(ctx context.Context, event domain.CustodyEvent, loan domain.LoanRequest, artifact domain.Artifact, loanVersion, artifactVersion int64) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCustody(ctx, tx, event); err != nil {
			return err
		}
		loanResult, err := tx.ExecContext(ctx, `
			UPDATE loan_requests
			SET status = ?, courier_reference = ?, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ?
		`, loan.Status, loan.CourierReference, timeText(loan.UpdatedAt),
			loan.TenantID, loan.ID, loanVersion)
		if err != nil {
			return fmt.Errorf("update loan custody state: %w", err)
		}
		if err := requireChanged(loanResult, "loan", domain.ErrVersion); err != nil {
			return err
		}
		artifactResult, err := tx.ExecContext(ctx, `
			UPDATE artifacts
			SET status = ?, active_loan_id = ?, current_zone_id = ?,
			    version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ?
		`, artifact.Status, nullableString(artifact.ActiveLoanID), artifact.CurrentZoneID,
			timeText(artifact.UpdatedAt), artifact.TenantID, artifact.ID, artifactVersion)
		if err != nil {
			return fmt.Errorf("update artifact custody state: %w", err)
		}
		return requireChanged(artifactResult, "artifact", domain.ErrVersion)
	})
}

func (s *Store) ApproveLoanAtomically(ctx context.Context, loan domain.LoanRequest, artifact domain.Artifact, audit domain.AuditEvent, expectedLoanVersion, expectedArtifactVersion int64) error {
	return s.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var activeIncidents int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM incidents
			WHERE tenant_id = ? AND artifact_id = ? AND status != ?
		`, loan.TenantID, loan.ArtifactID, domain.IncidentClosed).Scan(&activeIncidents); err != nil {
			return fmt.Errorf("check active incidents: %w", err)
		}
		if activeIncidents != 0 || artifact.Status != domain.ArtifactReady {
			return fmt.Errorf("approve loan: %w", domain.ErrPrecondition)
		}
		var overlaps int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM loan_requests
			WHERE tenant_id = ? AND artifact_id = ? AND id != ?
			  AND status IN (?, ?, ?, ?, ?) AND start_at < ? AND ? < end_at
		`, loan.TenantID, loan.ArtifactID, loan.ID,
			domain.LoanSubmitted, domain.LoanApproved, domain.LoanPacked,
			domain.LoanDispatched, domain.LoanReturning,
			timeText(loan.EndAt), timeText(loan.StartAt)).Scan(&overlaps); err != nil {
			return fmt.Errorf("check overlapping loans: %w", err)
		}
		if overlaps != 0 {
			return fmt.Errorf("approve loan: %w", domain.ErrConflict)
		}
		loanResult, err := tx.ExecContext(ctx, `
			UPDATE loan_requests
			SET status = ?, approved_by = ?, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ? AND status = ?
		`, domain.LoanApproved, loan.ApprovedBy, timeText(s.Now()), loan.TenantID,
			loan.ID, expectedLoanVersion, domain.LoanSubmitted)
		if err != nil {
			return fmt.Errorf("approve loan: %w", err)
		}
		if err := requireChanged(loanResult, "loan", domain.ErrVersion); err != nil {
			return err
		}
		artifactResult, err := tx.ExecContext(ctx, `
			UPDATE artifacts SET active_loan_id = ?, version = version + 1, updated_at = ?
			WHERE tenant_id = ? AND id = ? AND version = ? AND status = ? AND active_loan_id IS NULL
		`, loan.ID, timeText(s.Now()), artifact.TenantID, artifact.ID,
			expectedArtifactVersion, domain.ArtifactReady)
		if err != nil {
			return fmt.Errorf("reserve artifact for loan: %w", err)
		}
		if err := requireChanged(artifactResult, "artifact", domain.ErrConflict); err != nil {
			return err
		}
		return insertAudit(ctx, tx, audit)
	})
}
