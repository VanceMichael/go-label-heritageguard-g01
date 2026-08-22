package exhibition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/eventbus"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

const EnvironmentAssessmentJob = "environment.assess"

type Service struct {
	Artifacts repository.ArtifactRepository
	Cases     repository.ExhibitionRepository
	Events    *eventbus.Bus
	IDs       service.IDGenerator
	Now       func() time.Time
}

type ReservationInput struct {
	ArtifactID    string        `json:"artifact_id"`
	DisplayCaseID string        `json:"display_case_id"`
	Duration      time.Duration `json:"-"`
}

func (s *Service) Reserve(ctx context.Context, input ReservationInput) (domain.DisplayCase, error) {
	principal, err := service.RequireRole(ctx, domain.RoleCoordinator, domain.RoleSupervisor)
	if err != nil {
		return domain.DisplayCase{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, input.ArtifactID)
	if err != nil {
		return domain.DisplayCase{}, fmt.Errorf("load artifact for display reservation: %w", err)
	}
	if artifact.Status != domain.ArtifactReady || artifact.CurrentCaseID != "" || artifact.ActiveLoanID != "" {
		return domain.DisplayCase{}, fmt.Errorf("artifact is not eligible for display: %w", domain.ErrPrecondition)
	}
	displayCase, err := s.Cases.GetDisplayCase(ctx, principal.TenantID, input.DisplayCaseID)
	if err != nil {
		return domain.DisplayCase{}, fmt.Errorf("load display case: %w", err)
	}
	if displayCase.Status != domain.CaseAvailable || displayCase.ArtifactID != "" {
		return domain.DisplayCase{}, fmt.Errorf("display case unavailable: %w", domain.ErrConflict)
	}
	if input.Duration <= 0 || input.Duration > 72*time.Hour {
		return domain.DisplayCase{}, domain.FieldError{Field: "reservation_duration", Message: "must be between zero and 72 hours"}
	}
	expectedVersion := displayCase.Version
	reservationTo := s.Now().Add(input.Duration)
	displayCase.ArtifactID = artifact.ID
	displayCase.Status = domain.CaseReserved
	displayCase.ReservationTo = &reservationTo
	if err := s.Cases.ReserveDisplayCase(ctx, displayCase, expectedVersion); err != nil {
		return domain.DisplayCase{}, fmt.Errorf("reserve display case: %w", err)
	}
	displayCase.Version++
	displayCase.UpdatedAt = s.Now().UTC()
	return displayCase, nil
}

type InstallationInput struct {
	ArtifactID       string `json:"artifact_id"`
	DisplayCaseID    string `json:"display_case_id"`
	MountVerified    bool   `json:"mount_verified"`
	SealVerified     bool   `json:"seal_verified"`
	EnvironmentReady bool   `json:"environment_ready"`
}

func (s *Service) Activate(ctx context.Context, input InstallationInput) (domain.Installation, error) {
	principal, err := service.RequireRole(ctx, domain.RoleCoordinator, domain.RoleSupervisor)
	if err != nil {
		return domain.Installation{}, err
	}
	artifact, err := s.Artifacts.GetArtifact(ctx, principal.TenantID, input.ArtifactID)
	if err != nil {
		return domain.Installation{}, err
	}
	displayCase, err := s.Cases.GetDisplayCase(ctx, principal.TenantID, input.DisplayCaseID)
	if err != nil {
		return domain.Installation{}, err
	}
	if artifact.Status != domain.ArtifactReady || displayCase.Status != domain.CaseReserved || displayCase.ArtifactID != artifact.ID {
		return domain.Installation{}, fmt.Errorf("reservation no longer matches artifact: %w", domain.ErrPrecondition)
	}
	installation := domain.Installation{
		ID:               s.IDs.New("installation"),
		TenantID:         principal.TenantID,
		ArtifactID:       artifact.ID,
		DisplayCaseID:    displayCase.ID,
		MountVerified:    input.MountVerified,
		SealVerified:     input.SealVerified,
		EnvironmentReady: input.EnvironmentReady,
		InstalledBy:      principal.UserID,
		InstalledAt:      s.Now().UTC(),
	}
	if !installation.Complete() {
		return domain.Installation{}, fmt.Errorf("installation checklist incomplete: %w", domain.ErrPrecondition)
	}
	if err := s.Cases.ActivateInstallation(ctx, installation, artifact, displayCase, artifact.Version, displayCase.Version); err != nil {
		return domain.Installation{}, fmt.Errorf("activate display installation: %w", err)
	}
	return installation, nil
}

type ReadingInput struct {
	DisplayCaseID string    `json:"display_case_id"`
	DeviceID      string    `json:"device_id"`
	Sequence      int64     `json:"sequence"`
	TemperatureC  float64   `json:"temperature_c"`
	Humidity      float64   `json:"humidity"`
	ObservedAt    time.Time `json:"observed_at"`
}

func (s *Service) RecordReading(ctx context.Context, tenantID string, input ReadingInput) (domain.EnvironmentReading, error) {
	if err := ctx.Err(); err != nil {
		return domain.EnvironmentReading{}, err
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(input.DisplayCaseID) == "" || strings.TrimSpace(input.DeviceID) == "" {
		return domain.EnvironmentReading{}, domain.FieldError{Field: "reading", Message: "tenant, case and device are required"}
	}
	if input.Sequence < 0 || input.Humidity < 0 || input.Humidity > 100 {
		return domain.EnvironmentReading{}, domain.FieldError{Field: "reading", Message: "invalid sequence or humidity"}
	}
	if input.ObservedAt.IsZero() {
		return domain.EnvironmentReading{}, domain.FieldError{Field: "observed_at", Message: "required"}
	}
	if _, err := s.Cases.GetDisplayCase(ctx, tenantID, input.DisplayCaseID); err != nil {
		return domain.EnvironmentReading{}, err
	}
	reading := domain.EnvironmentReading{
		ID:            s.IDs.New("reading"),
		TenantID:      tenantID,
		DisplayCaseID: input.DisplayCaseID,
		DeviceID:      input.DeviceID,
		Sequence:      input.Sequence,
		TemperatureC:  input.TemperatureC,
		Humidity:      input.Humidity,
		ObservedAt:    input.ObservedAt.UTC(),
		ReceivedAt:    s.Now().UTC(),
	}
	payload, err := json.Marshal(map[string]any{
		"display_case_id":    reading.DisplayCaseID,
		"trigger_reading_id": reading.ID,
		"window":             "15m",
	})
	if err != nil {
		return domain.EnvironmentReading{}, fmt.Errorf("encode environment assessment job: %w", err)
	}
	job := domain.WorkerJob{
		ID:          s.IDs.New("job"),
		TenantID:    tenantID,
		Kind:        EnvironmentAssessmentJob,
		AggregateID: reading.ID,
		Payload:     payload,
		Status:      domain.JobPending,
		MaxAttempts: 5,
		AvailableAt: reading.ReceivedAt,
		CreatedAt:   reading.ReceivedAt,
		UpdatedAt:   reading.ReceivedAt,
	}
	if err := s.Cases.SaveReadingAndEnqueue(ctx, reading, job); err != nil {
		return domain.EnvironmentReading{}, err
	}
	return reading, nil
}

func (s *Service) Assess(ctx context.Context, caseID string, window time.Duration) (domain.EnvironmentAssessment, error) {
	principal, err := service.RequireRole(ctx, domain.RoleCoordinator, domain.RoleConservator, domain.RoleSupervisor)
	if err != nil {
		return domain.EnvironmentAssessment{}, err
	}
	if window < 5*time.Minute || window > 30*24*time.Hour {
		return domain.EnvironmentAssessment{}, domain.FieldError{Field: "window", Message: "must be between five minutes and thirty days"}
	}
	assessment, err := s.assessForTenant(ctx, principal.TenantID, caseID, window)
	if err != nil {
		return domain.EnvironmentAssessment{}, fmt.Errorf("assess display environment: %w", err)
	}
	return assessment, nil
}

func (s *Service) assessForTenant(ctx context.Context, tenantID, caseID string, window time.Duration) (domain.EnvironmentAssessment, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(caseID) == "" {
		return domain.EnvironmentAssessment{}, domain.FieldError{Field: "assessment", Message: "tenant and display case are required"}
	}
	if window < 5*time.Minute || window > 30*24*time.Hour {
		return domain.EnvironmentAssessment{}, domain.FieldError{Field: "window", Message: "must be between five minutes and thirty days"}
	}
	end := s.Now().UTC()
	// Propagate the caller's cancellation instead of detaching it. When the
	// request is cancelled during the reading window, AssessEnvironment
	// observes ctx.Err() while iterating and returns an error rather than
	// surfacing partial readings as a complete go/no-go decision.
	return s.Cases.AssessEnvironment(ctx, tenantID, caseID, end.Add(-window), end)
}

type IncidentInput struct {
	DisplayCaseID string `json:"display_case_id"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	WindowKey     string `json:"window_key"`
	Kind          string `json:"kind"`
	Summary       string `json:"summary"`
}

func (s *Service) OpenIncident(ctx context.Context, tenantID string, input IncidentInput) (domain.Incident, error) {
	if err := ctx.Err(); err != nil {
		return domain.Incident{}, err
	}
	if strings.TrimSpace(input.DisplayCaseID) == "" || strings.TrimSpace(input.WindowKey) == "" || strings.TrimSpace(input.Kind) == "" {
		return domain.Incident{}, domain.FieldError{Field: "incident", Message: "case, window and kind are required"}
	}
	now := s.Now().UTC()
	incident := domain.Incident{
		ID:            s.IDs.New("incident"),
		TenantID:      tenantID,
		DisplayCaseID: input.DisplayCaseID,
		ArtifactID:    input.ArtifactID,
		WindowKey:     input.WindowKey,
		Kind:          input.Kind,
		Status:        domain.IncidentOpen,
		Summary:       strings.TrimSpace(input.Summary),
		Version:       1,
		OpenedAt:      now,
		UpdatedAt:     now,
	}
	payload, err := json.Marshal(incident)
	if err != nil {
		return domain.Incident{}, fmt.Errorf("encode incident outbox event: %w", err)
	}
	event := domain.OutboxEvent{
		ID:             s.IDs.New("outbox"),
		TenantID:       tenantID,
		Topic:          "exhibition.incident.opened",
		AggregateID:    incident.ID,
		IdempotencyKey: incident.DisplayCaseID + ":" + incident.WindowKey + ":" + incident.Kind,
		Payload:        payload,
		Status:         domain.JobPending,
		MaxAttempts:    8,
		AvailableAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	stored, created, err := s.Cases.OpenIncidentAndOutbox(ctx, incident, event)
	if err != nil {
		return domain.Incident{}, err
	}
	if created && s.Events != nil {
		if err := s.Events.Publish(ctx, eventbus.Event{Topic: event.Topic, Key: stored.ID, Body: payload}); err != nil {
			return domain.Incident{}, fmt.Errorf("publish incident event: %w", err)
		}
	}
	return stored, nil
}

func (s *Service) AdvanceIncident(ctx context.Context, id string, to domain.IncidentStatus, remediated bool) (domain.Incident, error) {
	principal, err := service.RequireRole(ctx, domain.RoleCoordinator, domain.RoleConservator, domain.RoleSupervisor)
	if err != nil {
		return domain.Incident{}, err
	}
	incident, err := s.Cases.GetIncident(ctx, principal.TenantID, id)
	if err != nil {
		return domain.Incident{}, err
	}
	incident.Remediated = remediated
	if err := incident.CanTransition(to); err != nil {
		return domain.Incident{}, err
	}
	expectedVersion := incident.Version
	incident.Status = to
	incident.UpdatedAt = s.Now().UTC()
	if to == domain.IncidentClosed {
		closedAt := incident.UpdatedAt
		incident.ClosedAt = &closedAt
	}
	if err := s.Cases.UpdateIncident(ctx, incident, expectedVersion); err != nil {
		return domain.Incident{}, err
	}
	incident.Version++
	return incident, nil
}
