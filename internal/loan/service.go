package loan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

type AtomicApprover interface {
	ApproveLoanAtomically(context.Context, domain.LoanRequest, domain.Artifact, domain.AuditEvent, int64, int64) error
}

type Service struct {
	Artifacts repository.ArtifactRepository
	Loans     repository.LoanRepository
	Approver  AtomicApprover
	IDs       service.IDGenerator
	Now       func() time.Time
}

type CreateInput struct {
	ArtifactID string    `json:"artifact_id"`
	Borrower   string    `json:"borrower"`
	Purpose    string    `json:"purpose"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
}

func (s *Service) Create(ctx context.Context, input CreateInput) (domain.LoanRequest, error) {
	principal, err := service.RequireRole(ctx, domain.RoleRegistrar, domain.RoleCoordinator, domain.RoleSupervisor)
	if err != nil {
		return domain.LoanRequest{}, err
	}
	if strings.TrimSpace(input.Borrower) == "" || strings.TrimSpace(input.Purpose) == "" {
		return domain.LoanRequest{}, domain.FieldError{Field: "loan", Message: "borrower and purpose are required"}
	}
	if input.StartAt.Before(s.Now()) || !input.StartAt.Before(input.EndAt) {
		return domain.LoanRequest{}, domain.FieldError{Field: "loan_period", Message: "must be a future non-empty interval"}
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, input.ArtifactID)
	if err != nil {
		return domain.LoanRequest{}, fmt.Errorf("load artifact for loan: %w", err)
	}
	if artifact.Status != domain.ArtifactReady || artifact.CurrentCaseID != "" || artifact.ActiveLoanID != "" {
		return domain.LoanRequest{}, fmt.Errorf("artifact is not eligible for loan: %w", domain.ErrPrecondition)
	}
	activeIncident, err := s.Loans.HasActiveArtifactIncidents(ctx, principal.TenantID, artifact.ID)
	if err != nil {
		return domain.LoanRequest{}, fmt.Errorf("check artifact incidents: %w", err)
	}
	if activeIncident {
		return domain.LoanRequest{}, fmt.Errorf("artifact has an active incident: %w", domain.ErrPrecondition)
	}
	overlaps, err := s.Loans.ListOverlappingLoans(ctx, principal.TenantID, artifact.ID, input.StartAt, input.EndAt)
	if err != nil {
		return domain.LoanRequest{}, fmt.Errorf("check loan period: %w", err)
	}
	if len(overlaps) != 0 {
		return domain.LoanRequest{}, fmt.Errorf("loan period overlaps existing commitment: %w", domain.ErrConflict)
	}
	now := s.Now().UTC()
	item := domain.LoanRequest{
		ID:         s.IDs.New("loan"),
		TenantID:   principal.TenantID,
		ArtifactID: artifact.ID,
		Borrower:   strings.TrimSpace(input.Borrower),
		Purpose:    strings.TrimSpace(input.Purpose),
		StartAt:    input.StartAt.UTC(),
		EndAt:      input.EndAt.UTC(),
		Status:     domain.LoanDraft,
		Version:    1,
		CreatedBy:  principal.UserID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.Loans.CreateLoan(ctx, item); err != nil {
		return domain.LoanRequest{}, err
	}
	return item, nil
}

func (s *Service) Submit(ctx context.Context, id string) (domain.LoanRequest, error) {
	principal, err := service.RequireRole(ctx, domain.RoleRegistrar, domain.RoleCoordinator, domain.RoleSupervisor)
	if err != nil {
		return domain.LoanRequest{}, err
	}
	item, err := s.Loans.GetLoan(ctx, principal.TenantID, id)
	if err != nil {
		return domain.LoanRequest{}, err
	}
	if item.CreatedBy != principal.UserID && !principal.Can(domain.RoleSupervisor) {
		return domain.LoanRequest{}, domain.ErrForbidden
	}
	if err := item.CanTransition(domain.LoanSubmitted); err != nil {
		return domain.LoanRequest{}, err
	}
	expectedVersion := item.Version
	item.Status = domain.LoanSubmitted
	item.UpdatedAt = s.Now().UTC()
	if err := s.Loans.UpdateLoan(ctx, item, expectedVersion); err != nil {
		return domain.LoanRequest{}, err
	}
	item.Version++
	return item, nil
}

func (s *Service) Approve(ctx context.Context, id string) (domain.LoanRequest, error) {
	principal, err := service.RequireRole(ctx, domain.RoleSupervisor)
	if err != nil {
		return domain.LoanRequest{}, err
	}
	item, err := s.Loans.GetLoan(ctx, principal.TenantID, id)
	if err != nil {
		return domain.LoanRequest{}, err
	}
	if err := item.CanTransition(domain.LoanApproved); err != nil {
		return domain.LoanRequest{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, item.ArtifactID)
	if err != nil {
		return domain.LoanRequest{}, err
	}
	item.ApprovedBy = principal.UserID
	details, _ := json.Marshal(map[string]any{"borrower": item.Borrower, "start_at": item.StartAt, "end_at": item.EndAt})
	audit := domain.AuditEvent{
		ID:         s.IDs.New("audit"),
		TenantID:   principal.TenantID,
		ActorID:    principal.UserID,
		Action:     "loan.approve",
		ObjectType: "loan_request",
		ObjectID:   item.ID,
		Result:     "success",
		RequestID:  service.RequestIDFrom(ctx),
		Details:    details,
		CreatedAt:  s.Now().UTC(),
	}
	if err := s.Approver.ApproveLoanAtomically(ctx, item, artifact, audit, item.Version, artifact.Version); err != nil {
		return domain.LoanRequest{}, fmt.Errorf("approve loan transaction: %w", err)
	}
	item.Status = domain.LoanApproved
	item.Version++
	item.UpdatedAt = s.Now().UTC()
	return item, nil
}

type CustodyInput struct {
	LoanID     string `json:"loan_id"`
	FromHolder string `json:"from_holder"`
	ToHolder   string `json:"to_holder"`
	Location   string `json:"location"`
	SealNumber string `json:"seal_number"`
	Kind       string `json:"kind"`
}

func (s *Service) RecordCustody(ctx context.Context, input CustodyInput) (domain.CustodyEvent, error) {
	principal, err := service.RequireRole(ctx, domain.RoleCoordinator, domain.RoleSupervisor)
	if err != nil {
		return domain.CustodyEvent{}, err
	}
	item, err := s.Loans.GetLoan(ctx, principal.TenantID, input.LoanID)
	if err != nil {
		return domain.CustodyEvent{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, item.ArtifactID)
	if err != nil {
		return domain.CustodyEvent{}, err
	}
	to, artifactStatus, activeLoan, err := custodyTransition(item.Status, input.Kind, item.ID)
	if err != nil {
		return domain.CustodyEvent{}, err
	}
	event := domain.CustodyEvent{
		ID:         s.IDs.New("custody"),
		TenantID:   principal.TenantID,
		ArtifactID: artifact.ID,
		LoanID:     item.ID,
		FromHolder: strings.TrimSpace(input.FromHolder),
		ToHolder:   strings.TrimSpace(input.ToHolder),
		Location:   strings.TrimSpace(input.Location),
		SealNumber: strings.TrimSpace(input.SealNumber),
		Kind:       input.Kind,
		OccurredAt: s.Now().UTC(),
		RecordedBy: principal.UserID,
	}
	if event.FromHolder == "" || event.ToHolder == "" || event.Location == "" {
		return domain.CustodyEvent{}, domain.FieldError{Field: "custody", Message: "holders and location are required"}
	}
	loanVersion := item.Version
	artifactVersion := artifact.Version
	item.Status = to
	item.UpdatedAt = event.OccurredAt
	artifact.Status = artifactStatus
	artifact.ActiveLoanID = activeLoan
	artifact.CurrentZoneID = event.Location
	artifact.UpdatedAt = event.OccurredAt
	if err := s.Loans.RecordCustody(ctx, event, item, artifact, loanVersion, artifactVersion); err != nil {
		return domain.CustodyEvent{}, fmt.Errorf("record custody transaction: %w", err)
	}
	return event, nil
}

func custodyTransition(current domain.LoanStatus, kind, loanID string) (domain.LoanStatus, domain.ArtifactStatus, string, error) {
	switch kind {
	case "packed":
		if current != domain.LoanApproved {
			return "", "", "", domain.StateError{Entity: "loan_request", From: string(current), To: string(domain.LoanPacked), Reason: "loan must be approved before packing"}
		}
		return domain.LoanPacked, domain.ArtifactReady, loanID, nil
	case "dispatched":
		if current != domain.LoanPacked {
			return "", "", "", domain.StateError{Entity: "loan_request", From: string(current), To: string(domain.LoanDispatched), Reason: "sealed package must exist before dispatch"}
		}
		return domain.LoanDispatched, domain.ArtifactOnLoan, loanID, nil
	case "returning":
		if current != domain.LoanDispatched {
			return "", "", "", domain.StateError{Entity: "loan_request", From: string(current), To: string(domain.LoanReturning), Reason: "only dispatched loans can return"}
		}
		return domain.LoanReturning, domain.ArtifactOnLoan, loanID, nil
	case "returned":
		returningPhase := current == domain.LoanReturning
		dispatchedPhase := current == domain.LoanDispatched
		if !returningPhase && !dispatchedPhase {
			return "", "", "", domain.StateError{Entity: "loan_request", From: string(current), To: string(domain.LoanReturned), Reason: "return journey must be recorded first"}
		}
		return domain.LoanReturned, domain.ArtifactAssessment, "", nil
	default:
		return "", "", "", domain.FieldError{Field: "kind", Message: "unsupported custody transition"}
	}
}
