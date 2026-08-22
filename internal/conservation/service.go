package conservation

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

type Service struct {
	Artifacts repository.ArtifactRepository
	Cases     repository.ConservationRepository
	IDs       service.IDGenerator
	Now       func() time.Time
}

type RegisterArtifactInput struct {
	AccessionNo   string             `json:"accession_no"`
	Name          string             `json:"name"`
	Material      string             `json:"material"`
	Period        string             `json:"period"`
	RiskClass     domain.RiskClass   `json:"risk_class"`
	InitialZoneID string             `json:"initial_zone_id"`
	Summary       string             `json:"summary"`
	Measurements  map[string]float64 `json:"measurements"`
	Issues        []string           `json:"issues"`
}

type RegisterArtifactResult struct {
	Artifact domain.Artifact        `json:"artifact"`
	Report   domain.ConditionReport `json:"condition_report"`
	Custody  domain.CustodyEvent    `json:"custody_event"`
}

func (s *Service) Register(ctx context.Context, input RegisterArtifactInput) (RegisterArtifactResult, error) {
	principal, err := service.RequireRole(ctx, domain.RoleRegistrar, domain.RoleSupervisor)
	if err != nil {
		return RegisterArtifactResult{}, err
	}
	now := s.Now().UTC()
	artifact := domain.Artifact{
		ID:            s.IDs.New("artifact"),
		TenantID:      principal.TenantID,
		AccessionNo:   strings.TrimSpace(input.AccessionNo),
		Name:          strings.TrimSpace(input.Name),
		Material:      strings.TrimSpace(input.Material),
		Period:        strings.TrimSpace(input.Period),
		RiskClass:     input.RiskClass,
		Status:        domain.ArtifactRegistered,
		CurrentZoneID: strings.TrimSpace(input.InitialZoneID),
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := artifact.ValidateNew(); err != nil {
		return RegisterArtifactResult{}, err
	}
	if artifact.CurrentZoneID == "" {
		return RegisterArtifactResult{}, domain.FieldError{Field: "initial_zone_id", Message: "required"}
	}
	report := domain.ConditionReport{
		ID:             s.IDs.New("condition"),
		TenantID:       principal.TenantID,
		ArtifactID:     artifact.ID,
		InspectorID:    principal.UserID,
		Summary:        strings.TrimSpace(input.Summary),
		Severity:       input.RiskClass,
		Measurements:   cloneMap(input.Measurements),
		ObservedIssues: append([]string(nil), input.Issues...),
		Final:          true,
		CreatedAt:      now,
	}
	if report.Summary == "" {
		return RegisterArtifactResult{}, domain.FieldError{Field: "summary", Message: "initial condition summary is required"}
	}
	custody := domain.CustodyEvent{
		ID:         s.IDs.New("custody"),
		TenantID:   principal.TenantID,
		ArtifactID: artifact.ID,
		FromHolder: "external intake",
		ToHolder:   principal.UserID,
		Location:   artifact.CurrentZoneID,
		Kind:       "registered",
		OccurredAt: now,
		RecordedBy: principal.UserID,
	}
	audit := domain.AuditEvent{
		ID:         s.IDs.New("audit"),
		TenantID:   principal.TenantID,
		ActorID:    principal.UserID,
		Action:     "artifact.register",
		ObjectType: "artifact",
		ObjectID:   artifact.ID,
		Result:     "success",
		RequestID:  service.RequestIDFrom(ctx),
		Details:    json.RawMessage(`{"condition":"initial","custody":"registered"}`),
		CreatedAt:  now,
	}
	if err := s.Artifacts.CreateArtifact(ctx, artifact, report, custody, audit); err != nil {
		return RegisterArtifactResult{}, fmt.Errorf("register artifact transaction: %w", err)
	}
	artifact.LastReportID = report.ID
	return RegisterArtifactResult{Artifact: artifact, Report: report.Clone(), Custody: custody}, nil
}

type ConditionInput struct {
	ArtifactID   string             `json:"artifact_id"`
	Summary      string             `json:"summary"`
	Severity     domain.RiskClass   `json:"severity"`
	Measurements map[string]float64 `json:"measurements"`
	Issues       []string           `json:"issues"`
	Final        bool               `json:"final"`
}

func (s *Service) RecordCondition(ctx context.Context, input ConditionInput) (domain.ConditionReport, error) {
	principal, err := service.RequireRole(ctx, domain.RoleConservator, domain.RoleSupervisor)
	if err != nil {
		return domain.ConditionReport{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, input.ArtifactID)
	if err != nil {
		return domain.ConditionReport{}, fmt.Errorf("load artifact for condition report: %w", err)
	}
	if artifact.Status == domain.ArtifactArchived || artifact.Status == domain.ArtifactOnLoan {
		return domain.ConditionReport{}, fmt.Errorf("artifact cannot be inspected in status %s: %w", artifact.Status, domain.ErrPrecondition)
	}
	report := domain.ConditionReport{
		ID:             s.IDs.New("condition"),
		TenantID:       principal.TenantID,
		ArtifactID:     artifact.ID,
		InspectorID:    principal.UserID,
		Summary:        strings.TrimSpace(input.Summary),
		Severity:       input.Severity,
		Measurements:   cloneMap(input.Measurements),
		ObservedIssues: append([]string(nil), input.Issues...),
		Final:          input.Final,
		CreatedAt:      s.Now().UTC(),
	}
	if report.Summary == "" {
		return domain.ConditionReport{}, domain.FieldError{Field: "summary", Message: "required"}
	}
	if err := s.Artifacts.SaveConditionReport(ctx, report, artifact.Version); err != nil {
		return domain.ConditionReport{}, fmt.Errorf("save condition report: %w", err)
	}
	return report.Clone(), nil
}

func (s *Service) ReleaseAssessment(ctx context.Context, artifactID string) (domain.Artifact, error) {
	principal, err := service.RequireRole(ctx, domain.RoleConservator, domain.RoleSupervisor)
	if err != nil {
		return domain.Artifact{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, artifactID)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("load artifact assessment: %w", err)
	}
	if artifact.Status != domain.ArtifactAssessment || artifact.LastReportID == "" {
		return domain.Artifact{}, fmt.Errorf("artifact does not have an active assessment: %w", domain.ErrPrecondition)
	}
	report, err := s.Artifacts.GetConditionReport(ctx, principal.TenantID, artifact.LastReportID)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("load final condition report: %w", err)
	}
	if !report.Final {
		return domain.Artifact{}, fmt.Errorf("condition report is not final: %w", domain.ErrPrecondition)
	}
	if report.Severity == domain.RiskHigh || report.Severity == domain.RiskCritical {
		return domain.Artifact{}, fmt.Errorf("high-risk assessment requires quarantine: %w", domain.ErrPrecondition)
	}
	if err := artifact.CanTransition(domain.ArtifactReady); err != nil {
		return domain.Artifact{}, err
	}
	if err := s.Artifacts.UpdateArtifactStatus(ctx, artifact.TenantID, artifact.ID, domain.ArtifactReady, artifact.Version); err != nil {
		return domain.Artifact{}, fmt.Errorf("release artifact assessment: %w", err)
	}
	artifact.Status = domain.ArtifactReady
	artifact.Version++
	artifact.UpdatedAt = s.Now().UTC()
	return artifact, nil
}

type QuarantineInput struct {
	ArtifactID string `json:"artifact_id"`
	ZoneID     string `json:"zone_id"`
	Reason     string `json:"reason"`
}

func (s *Service) OpenQuarantine(ctx context.Context, input QuarantineInput) (domain.QuarantineCase, error) {
	principal, err := service.RequireRole(ctx, domain.RoleConservator, domain.RoleSupervisor)
	if err != nil {
		return domain.QuarantineCase{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, input.ArtifactID)
	if err != nil {
		return domain.QuarantineCase{}, err
	}
	if err := artifact.CanTransition(domain.ArtifactQuarantined); err != nil {
		return domain.QuarantineCase{}, err
	}
	if strings.TrimSpace(input.ZoneID) == "" || strings.TrimSpace(input.Reason) == "" {
		return domain.QuarantineCase{}, domain.FieldError{Field: "quarantine", Message: "zone and reason are required"}
	}
	item := domain.QuarantineCase{
		ID:         s.IDs.New("quarantine"),
		TenantID:   principal.TenantID,
		ArtifactID: artifact.ID,
		ZoneID:     input.ZoneID,
		Reason:     strings.TrimSpace(input.Reason),
		Status:     domain.QuarantineOpen,
		OpenedBy:   principal.UserID,
		Version:    1,
		OpenedAt:   s.Now().UTC(),
	}
	if err := s.Cases.OpenQuarantine(ctx, item, artifact, artifact.Version); err != nil {
		return domain.QuarantineCase{}, fmt.Errorf("open quarantine transaction: %w", err)
	}
	return item, nil
}

type TreatmentInput struct {
	ArtifactID   string `json:"artifact_id"`
	QuarantineID string `json:"quarantine_id"`
	Procedure    string `json:"procedure"`
}

func (s *Service) DraftTreatment(ctx context.Context, input TreatmentInput) (domain.TreatmentPlan, error) {
	principal, err := service.RequireRole(ctx, domain.RoleConservator)
	if err != nil {
		return domain.TreatmentPlan{}, err
	}
	quarantine, err := s.Cases.GetQuarantine(ctx, principal.TenantID, input.QuarantineID)
	if err != nil {
		return domain.TreatmentPlan{}, fmt.Errorf("load quarantine for treatment: %w", err)
	}
	if quarantine.ArtifactID != input.ArtifactID || quarantine.Status == domain.QuarantineResolved {
		return domain.TreatmentPlan{}, fmt.Errorf("quarantine does not cover artifact: %w", domain.ErrPrecondition)
	}
	if strings.TrimSpace(input.Procedure) == "" {
		return domain.TreatmentPlan{}, domain.FieldError{Field: "procedure", Message: "required"}
	}
	now := s.Now().UTC()
	plan := domain.TreatmentPlan{
		ID:            s.IDs.New("treatment"),
		TenantID:      principal.TenantID,
		ArtifactID:    input.ArtifactID,
		QuarantineID:  input.QuarantineID,
		ConservatorID: principal.UserID,
		Procedure:     strings.TrimSpace(input.Procedure),
		Status:        domain.TreatmentDraft,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Cases.CreateTreatmentPlan(ctx, plan); err != nil {
		return domain.TreatmentPlan{}, err
	}
	return plan, nil
}

func (s *Service) AdvanceTreatment(ctx context.Context, id string, to domain.TreatmentStatus, evidenceURI string) (domain.TreatmentPlan, error) {
	principal, err := service.PrincipalFrom(ctx)
	if err != nil {
		return domain.TreatmentPlan{}, err
	}
	plan, err := s.Cases.GetTreatmentPlan(ctx, principal.TenantID, id)
	if err != nil {
		return domain.TreatmentPlan{}, err
	}
	if err := plan.CanTransition(to); err != nil {
		return domain.TreatmentPlan{}, err
	}
	if to == domain.TreatmentApproved {
		if !principal.Can(domain.RoleSupervisor) {
			return domain.TreatmentPlan{}, domain.ErrForbidden
		}
		plan.ApprovedBy = principal.UserID
	} else if plan.ConservatorID != principal.UserID && !principal.Can(domain.RoleSupervisor) {
		return domain.TreatmentPlan{}, domain.ErrForbidden
	}
	if to == domain.TreatmentCompleted {
		if strings.TrimSpace(evidenceURI) == "" {
			return domain.TreatmentPlan{}, domain.FieldError{Field: "evidence_uri", Message: "required to complete treatment"}
		}
		plan.EvidenceURI = strings.TrimSpace(evidenceURI)
		completedAt := s.Now().UTC()
		plan.CompletedAt = &completedAt
	}
	expectedVersion := plan.Version
	plan.Status = to
	plan.UpdatedAt = s.Now().UTC()
	if err := s.Cases.UpdateTreatment(ctx, plan, expectedVersion); err != nil {
		return domain.TreatmentPlan{}, fmt.Errorf("advance treatment: %w", err)
	}
	plan.Version++
	return plan, nil
}

func cloneMap(input map[string]float64) map[string]float64 {
	output := make(map[string]float64, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
