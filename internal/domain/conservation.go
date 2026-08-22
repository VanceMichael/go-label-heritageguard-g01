package domain

import "time"

type QuarantineStatus string

const (
	QuarantineOpen     QuarantineStatus = "open"
	QuarantineTreating QuarantineStatus = "treating"
	QuarantineResolved QuarantineStatus = "resolved"
)

type QuarantineCase struct {
	ID         string           `json:"id"`
	TenantID   string           `json:"tenant_id"`
	ArtifactID string           `json:"artifact_id"`
	ZoneID     string           `json:"zone_id"`
	Reason     string           `json:"reason"`
	Status     QuarantineStatus `json:"status"`
	OpenedBy   string           `json:"opened_by"`
	ResolvedBy string           `json:"resolved_by,omitempty"`
	Version    int64            `json:"version"`
	OpenedAt   time.Time        `json:"opened_at"`
	ResolvedAt *time.Time       `json:"resolved_at,omitempty"`
}

type TreatmentStatus string

const (
	TreatmentDraft      TreatmentStatus = "draft"
	TreatmentApproved   TreatmentStatus = "approved"
	TreatmentInProgress TreatmentStatus = "in_progress"
	TreatmentCompleted  TreatmentStatus = "completed"
	TreatmentRejected   TreatmentStatus = "rejected"
)

type TreatmentPlan struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	ArtifactID    string          `json:"artifact_id"`
	QuarantineID  string          `json:"quarantine_id"`
	ConservatorID string          `json:"conservator_id"`
	Procedure     string          `json:"procedure"`
	EvidenceURI   string          `json:"evidence_uri,omitempty"`
	Status        TreatmentStatus `json:"status"`
	Version       int64           `json:"version"`
	ApprovedBy    string          `json:"approved_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

func (p TreatmentPlan) CanTransition(to TreatmentStatus) error {
	allowed := map[TreatmentStatus]map[TreatmentStatus]bool{
		TreatmentDraft:      {TreatmentApproved: true, TreatmentRejected: true},
		TreatmentApproved:   {TreatmentInProgress: true},
		TreatmentInProgress: {TreatmentCompleted: true},
		TreatmentCompleted:  {},
		TreatmentRejected:   {},
	}
	if allowed[p.Status][to] {
		return nil
	}
	return StateError{Entity: "treatment_plan", From: string(p.Status), To: string(to), Reason: "required treatment phase has not completed"}
}
