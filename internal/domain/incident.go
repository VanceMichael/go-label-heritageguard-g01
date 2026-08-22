package domain

import "time"

type IncidentStatus string

const (
	IncidentOpen       IncidentStatus = "open"
	IncidentResponding IncidentStatus = "responding"
	IncidentMonitoring IncidentStatus = "monitoring"
	IncidentClosed     IncidentStatus = "closed"
)

type Incident struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	DisplayCaseID string         `json:"display_case_id"`
	ArtifactID    string         `json:"artifact_id,omitempty"`
	WindowKey     string         `json:"window_key"`
	Kind          string         `json:"kind"`
	Status        IncidentStatus `json:"status"`
	Summary       string         `json:"summary"`
	Remediated    bool           `json:"remediated"`
	Version       int64          `json:"version"`
	OpenedAt      time.Time      `json:"opened_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	ClosedAt      *time.Time     `json:"closed_at,omitempty"`
}

func (i Incident) CanTransition(to IncidentStatus) error {
	allowed := map[IncidentStatus]map[IncidentStatus]bool{
		IncidentOpen:       {IncidentResponding: true},
		IncidentResponding: {IncidentMonitoring: true},
		IncidentMonitoring: {IncidentClosed: true, IncidentResponding: true},
		IncidentClosed:     {},
	}
	if !allowed[i.Status][to] {
		return StateError{Entity: "incident", From: string(i.Status), To: string(to), Reason: "response phases must be completed in order"}
	}
	if to == IncidentClosed && !i.Remediated {
		return StateError{Entity: "incident", From: string(i.Status), To: string(to), Reason: "remediation has not been verified"}
	}
	return nil
}
