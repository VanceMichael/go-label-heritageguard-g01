package repository

import (
	"context"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type UserRepository interface {
	CreateUser(context.Context, domain.User) error
	FindUserByEmail(context.Context, string, string) (domain.User, error)
	FindUser(context.Context, string, string) (domain.User, error)
	SetUserActive(context.Context, string, string, bool, int64) error
	ListActiveSessions(context.Context, string, string) ([]domain.Session, error)
}

type SessionRepository interface {
	CreateSession(context.Context, domain.Session) error
	FindSessionByToken(context.Context, []byte) (domain.Session, error)
	RevokeSession(context.Context, string, string, time.Time) error
	RevokeUserSessions(context.Context, string, string, time.Time) error
}

type ArtifactRepository interface {
	CreateArtifact(context.Context, domain.Artifact, domain.ConditionReport, domain.CustodyEvent, domain.AuditEvent) error
	GetArtifact(context.Context, string, string) (domain.Artifact, error)
	UpdateArtifactStatus(context.Context, string, string, domain.ArtifactStatus, int64) error
	ListArtifacts(context.Context, string, domain.Page, string) ([]domain.Artifact, int, error)
	SaveConditionReport(context.Context, domain.ConditionReport, int64) error
	GetConditionReport(context.Context, string, string) (domain.ConditionReport, error)
}

type ConservationRepository interface {
	OpenQuarantine(context.Context, domain.QuarantineCase, domain.Artifact, int64) error
	GetQuarantine(context.Context, string, string) (domain.QuarantineCase, error)
	CreateTreatmentPlan(context.Context, domain.TreatmentPlan) error
	GetTreatmentPlan(context.Context, string, string) (domain.TreatmentPlan, error)
	UpdateTreatment(context.Context, domain.TreatmentPlan, int64) error
}

type ExhibitionRepository interface {
	GetDisplayCase(context.Context, string, string) (domain.DisplayCase, error)
	ReserveDisplayCase(context.Context, domain.DisplayCase, int64) error
	ActivateInstallation(context.Context, domain.Installation, domain.Artifact, domain.DisplayCase, int64, int64) error
	SaveReadingAndEnqueue(context.Context, domain.EnvironmentReading, domain.WorkerJob) error
	AssessEnvironment(context.Context, string, string, time.Time, time.Time) (domain.EnvironmentAssessment, error)
	OpenIncidentAndOutbox(context.Context, domain.Incident, domain.OutboxEvent) (domain.Incident, bool, error)
	GetIncident(context.Context, string, string) (domain.Incident, error)
	UpdateIncident(context.Context, domain.Incident, int64) error
}

type LoanRepository interface {
	CreateLoan(context.Context, domain.LoanRequest) error
	GetLoan(context.Context, string, string) (domain.LoanRequest, error)
	UpdateLoan(context.Context, domain.LoanRequest, int64) error
	ListOverlappingLoans(context.Context, string, string, time.Time, time.Time) ([]domain.LoanRequest, error)
	HasActiveArtifactIncidents(context.Context, string, string) (bool, error)
	RecordCustody(context.Context, domain.CustodyEvent, domain.LoanRequest, domain.Artifact, int64, int64) error
}

type AuditRepository interface {
	Append(context.Context, domain.AuditEvent) error
	List(context.Context, string, string, domain.Page) ([]domain.AuditEvent, error)
}

type WorkerRepository interface {
	ClaimJob(context.Context, string, time.Time, time.Duration) (domain.WorkerJob, error)
	CompleteJob(context.Context, string, string, time.Time) error
	RetryJob(context.Context, string, string, time.Time, time.Time, error) error
	FailJob(context.Context, string, string, time.Time, error) error
	ReleaseLease(context.Context, string, string, time.Time) error
}
