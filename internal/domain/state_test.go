package domain

import (
	"errors"
	"testing"
	"time"
)

func TestArtifactValidateNew(t *testing.T) {
	valid := Artifact{
		TenantID: "museum-demo", AccessionNo: "NX-001", Name: "Painted brick",
		Material: "clay", RiskClass: RiskModerate,
	}
	tests := []struct {
		name    string
		mutate  func(*Artifact)
		wantErr bool
	}{
		{name: "valid"},
		{name: "tenant required", mutate: func(item *Artifact) { item.TenantID = " " }, wantErr: true},
		{name: "accession required", mutate: func(item *Artifact) { item.AccessionNo = "" }, wantErr: true},
		{name: "name required", mutate: func(item *Artifact) { item.Name = "" }, wantErr: true},
		{name: "material required", mutate: func(item *Artifact) { item.Material = "" }, wantErr: true},
		{name: "risk class required", mutate: func(item *Artifact) { item.RiskClass = "unknown" }, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := valid
			if test.mutate != nil {
				test.mutate(&item)
			}
			err := item.ValidateNew()
			if test.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid input, got %v", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("expected valid artifact, got %v", err)
			}
		})
	}
}

func TestArtifactTransitions(t *testing.T) {
	tests := []struct {
		from    ArtifactStatus
		to      ArtifactStatus
		allowed bool
	}{
		{ArtifactRegistered, ArtifactAssessment, true},
		{ArtifactRegistered, ArtifactQuarantined, true},
		{ArtifactAssessment, ArtifactReady, true},
		{ArtifactAssessment, ArtifactQuarantined, true},
		{ArtifactQuarantined, ArtifactTreatment, true},
		{ArtifactTreatment, ArtifactReady, true},
		{ArtifactTreatment, ArtifactQuarantined, true},
		{ArtifactReady, ArtifactOnDisplay, true},
		{ArtifactReady, ArtifactOnLoan, true},
		{ArtifactReady, ArtifactQuarantined, true},
		{ArtifactReady, ArtifactArchived, true},
		{ArtifactOnDisplay, ArtifactReady, true},
		{ArtifactOnDisplay, ArtifactQuarantined, true},
		{ArtifactOnLoan, ArtifactReady, true},
		{ArtifactOnLoan, ArtifactQuarantined, true},
		{ArtifactRegistered, ArtifactOnLoan, false},
		{ArtifactAssessment, ArtifactOnDisplay, false},
		{ArtifactQuarantined, ArtifactReady, false},
		{ArtifactArchived, ArtifactReady, false},
	}
	for _, test := range tests {
		name := string(test.from) + "_to_" + string(test.to)
		t.Run(name, func(t *testing.T) {
			err := (Artifact{Status: test.from}).CanTransition(test.to)
			if test.allowed && err != nil {
				t.Fatalf("expected transition to be allowed: %v", err)
			}
			if !test.allowed && !errors.Is(err, ErrIllegalState) {
				t.Fatalf("expected illegal state, got %v", err)
			}
		})
	}
}

func TestConditionReportCloneIsIndependent(t *testing.T) {
	original := ConditionReport{
		Measurements:   map[string]float64{"humidity": 49.2},
		ObservedIssues: []string{"surface dust"},
	}
	cloned := original.Clone()
	cloned.Measurements["humidity"] = 70
	cloned.ObservedIssues[0] = "changed"
	if original.Measurements["humidity"] != 49.2 {
		t.Fatal("clone mutated original measurements")
	}
	if original.ObservedIssues[0] != "surface dust" {
		t.Fatal("clone mutated original issues")
	}
}

func TestTreatmentTransitions(t *testing.T) {
	allowed := []struct {
		from TreatmentStatus
		to   TreatmentStatus
	}{
		{TreatmentDraft, TreatmentApproved},
		{TreatmentDraft, TreatmentRejected},
		{TreatmentApproved, TreatmentInProgress},
		{TreatmentInProgress, TreatmentCompleted},
	}
	for _, transition := range allowed {
		if err := (TreatmentPlan{Status: transition.from}).CanTransition(transition.to); err != nil {
			t.Errorf("expected %s to %s: %v", transition.from, transition.to, err)
		}
	}
	blocked := []struct {
		from TreatmentStatus
		to   TreatmentStatus
	}{
		{TreatmentDraft, TreatmentCompleted},
		{TreatmentApproved, TreatmentCompleted},
		{TreatmentCompleted, TreatmentInProgress},
		{TreatmentRejected, TreatmentApproved},
	}
	for _, transition := range blocked {
		if err := (TreatmentPlan{Status: transition.from}).CanTransition(transition.to); !errors.Is(err, ErrIllegalState) {
			t.Errorf("expected %s to %s blocked, got %v", transition.from, transition.to, err)
		}
	}
}

func TestLoanTransitionsAndOverlap(t *testing.T) {
	path := []LoanStatus{
		LoanDraft, LoanSubmitted, LoanApproved, LoanPacked,
		LoanDispatched, LoanReturning, LoanReturned,
	}
	for index := 0; index < len(path)-1; index++ {
		if err := (LoanRequest{Status: path[index]}).CanTransition(path[index+1]); err != nil {
			t.Fatalf("expected %s to %s: %v", path[index], path[index+1], err)
		}
	}
	if err := (LoanRequest{Status: LoanDraft}).CanTransition(LoanDispatched); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("expected skipped phases to fail, got %v", err)
	}
	start := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	loan := LoanRequest{StartAt: start, EndAt: start.Add(24 * time.Hour)}
	tests := []struct {
		name     string
		start    time.Time
		end      time.Time
		overlaps bool
	}{
		{name: "inside", start: start.Add(time.Hour), end: start.Add(2 * time.Hour), overlaps: true},
		{name: "contains", start: start.Add(-time.Hour), end: start.Add(25 * time.Hour), overlaps: true},
		{name: "touches start", start: start.Add(-time.Hour), end: start, overlaps: false},
		{name: "touches end", start: start.Add(24 * time.Hour), end: start.Add(25 * time.Hour), overlaps: false},
		{name: "before", start: start.Add(-3 * time.Hour), end: start.Add(-2 * time.Hour), overlaps: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := loan.Overlaps(test.start, test.end); got != test.overlaps {
				t.Fatalf("expected overlap %v, got %v", test.overlaps, got)
			}
		})
	}
}

func TestIncidentTransitionsRequireRemediation(t *testing.T) {
	incident := Incident{Status: IncidentOpen}
	if err := incident.CanTransition(IncidentResponding); err != nil {
		t.Fatalf("open to responding: %v", err)
	}
	incident.Status = IncidentResponding
	if err := incident.CanTransition(IncidentMonitoring); err != nil {
		t.Fatalf("responding to monitoring: %v", err)
	}
	incident.Status = IncidentMonitoring
	if err := incident.CanTransition(IncidentClosed); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("expected closure without remediation to fail, got %v", err)
	}
	incident.Remediated = true
	if err := incident.CanTransition(IncidentClosed); err != nil {
		t.Fatalf("expected remediated incident to close: %v", err)
	}
	if err := (Incident{Status: IncidentOpen}).CanTransition(IncidentClosed); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("expected phase skipping to fail, got %v", err)
	}
}

func TestRoleAndPrincipalAuthorization(t *testing.T) {
	roles := []Role{RoleRegistrar, RoleConservator, RoleCoordinator, RoleSupervisor}
	for _, role := range roles {
		if !role.Valid() {
			t.Errorf("expected role %s to be valid", role)
		}
		principal := Principal{Role: role}
		if !principal.Can(role) {
			t.Errorf("principal should have its own role %s", role)
		}
	}
	if Role("visitor").Valid() {
		t.Fatal("visitor must not be a valid operational role")
	}
	if (Principal{Role: RoleRegistrar}).Can(RoleSupervisor) {
		t.Fatal("registrar must not inherit supervisor privileges")
	}
}

func TestPageNormalize(t *testing.T) {
	tests := []struct {
		input Page
		want  Page
	}{
		{input: Page{}, want: Page{Limit: 25}},
		{input: Page{Limit: -1, Offset: -5}, want: Page{Limit: 25}},
		{input: Page{Limit: 101, Offset: 3}, want: Page{Limit: 100, Offset: 3}},
		{input: Page{Limit: 40, Offset: 10}, want: Page{Limit: 40, Offset: 10}},
	}
	for _, test := range tests {
		if got := test.input.Normalize(); got != test.want {
			t.Errorf("normalize %+v: got %+v, want %+v", test.input, got, test.want)
		}
	}
}

func TestStructuredErrorsUnwrap(t *testing.T) {
	if !errors.Is(FieldError{Field: "name", Message: "required"}, ErrInvalid) {
		t.Fatal("field error should unwrap to invalid")
	}
	if !errors.Is(StateError{Entity: "loan", From: "draft", To: "returned", Reason: "skipped"}, ErrIllegalState) {
		t.Fatal("state error should unwrap to illegal state")
	}
	underlying := errors.New("dial failed")
	dependency := DependencyError{Operation: "notify", Err: underlying}
	if !errors.Is(dependency, ErrUnavailable) || !errors.Is(dependency, underlying) {
		t.Fatal("dependency error should expose availability and underlying cause")
	}
}
