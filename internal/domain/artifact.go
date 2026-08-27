package domain

import (
	"fmt"
	"strings"
	"time"
)

type ArtifactStatus string

const (
	ArtifactRegistered  ArtifactStatus = "registered"
	ArtifactAssessment  ArtifactStatus = "assessment"
	ArtifactQuarantined ArtifactStatus = "quarantined"
	ArtifactTreatment   ArtifactStatus = "treatment"
	ArtifactReady       ArtifactStatus = "ready"
	ArtifactOnDisplay   ArtifactStatus = "on_display"
	ArtifactOnLoan      ArtifactStatus = "on_loan"
	ArtifactArchived    ArtifactStatus = "archived"
)

type RiskClass string

const (
	RiskLow      RiskClass = "low"
	RiskModerate RiskClass = "moderate"
	RiskHigh     RiskClass = "high"
	RiskCritical RiskClass = "critical"
)

type Artifact struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	AccessionNo   string         `json:"accession_no"`
	Name          string         `json:"name"`
	Material      string         `json:"material"`
	Period        string         `json:"period"`
	RiskClass     RiskClass      `json:"risk_class"`
	Status        ArtifactStatus `json:"status"`
	CurrentZoneID string         `json:"current_zone_id"`
	CurrentCaseID string         `json:"current_case_id,omitempty"`
	ActiveLoanID  string         `json:"active_loan_id,omitempty"`
	LastReportID  string         `json:"last_report_id,omitempty"`
	Version       int64          `json:"version"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (a Artifact) ValidateNew() error {
	if err := Require(strings.TrimSpace(a.TenantID) != "", "tenant_id", "required"); err != nil {
		return err
	}
	if err := Require(strings.TrimSpace(a.AccessionNo) != "", "accession_no", "required"); err != nil {
		return err
	}
	if err := Require(strings.TrimSpace(a.Name) != "", "name", "required"); err != nil {
		return err
	}
	if err := Require(strings.TrimSpace(a.Material) != "", "material", "required"); err != nil {
		return err
	}
	switch a.RiskClass {
	case RiskLow, RiskModerate, RiskHigh, RiskCritical:
	default:
		return FieldError{Field: "risk_class", Message: fmt.Sprintf("unsupported value %q", a.RiskClass)}
	}
	return nil
}

func (a Artifact) CanTransition(to ArtifactStatus) error {
	allowed := map[ArtifactStatus]map[ArtifactStatus]bool{
		ArtifactRegistered:  {ArtifactAssessment: true, ArtifactQuarantined: true},
		ArtifactAssessment:  {ArtifactReady: true, ArtifactQuarantined: true},
		ArtifactQuarantined: {ArtifactTreatment: true},
		ArtifactTreatment:   {ArtifactReady: true, ArtifactQuarantined: true},
		ArtifactReady:       {ArtifactOnDisplay: true, ArtifactOnLoan: true, ArtifactQuarantined: true, ArtifactArchived: true},
		ArtifactOnDisplay:   {ArtifactReady: true, ArtifactQuarantined: true},
		ArtifactOnLoan:      {ArtifactReady: true, ArtifactQuarantined: true},
		ArtifactArchived:    {},
	}
	if allowed[a.Status][to] {
		return nil
	}
	return StateError{Entity: "artifact", From: string(a.Status), To: string(to), Reason: "transition is not allowed by conservation policy"}
}

type ConditionReport struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenant_id"`
	ArtifactID     string             `json:"artifact_id"`
	InspectorID    string             `json:"inspector_id"`
	Summary        string             `json:"summary"`
	Severity       RiskClass          `json:"severity"`
	Measurements   map[string]float64 `json:"measurements"`
	ObservedIssues []string           `json:"observed_issues"`
	Final          bool               `json:"final"`
	CreatedAt      time.Time          `json:"created_at"`
}

func (r ConditionReport) Clone() ConditionReport {
	copyReport := r
	copyReport.Measurements = make(map[string]float64, len(r.Measurements))
	for key, value := range r.Measurements {
		copyReport.Measurements[key] = value
	}
	copyReport.ObservedIssues = append([]string(nil), r.ObservedIssues...)
	return copyReport
}
