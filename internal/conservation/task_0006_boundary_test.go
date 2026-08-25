package conservation

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

type task0006Artifacts struct{ saved domain.ConditionReport }

func (s *task0006Artifacts) CreateArtifact(context.Context, domain.Artifact, domain.ConditionReport, domain.CustodyEvent, domain.AuditEvent) error { return nil }
func (s *task0006Artifacts) GetArtifact(context.Context, string, string) (domain.Artifact, error) { return domain.Artifact{ID: "artifact-task-0006", TenantID: "museum-demo", Status: domain.ArtifactReady, Version: 1}, nil }
func (s *task0006Artifacts) UpdateArtifactStatus(context.Context, string, string, domain.ArtifactStatus, int64) error { return nil }
func (s *task0006Artifacts) ListArtifacts(context.Context, string, domain.Page, string) ([]domain.Artifact, int, error) { return nil, 0, nil }
func (s *task0006Artifacts) SaveConditionReport(_ context.Context, report domain.ConditionReport, _ int64) error { s.saved = report; return nil }
func (s *task0006Artifacts) GetConditionReport(context.Context, string, string) (domain.ConditionReport, error) { return domain.ConditionReport{}, nil }

func TestHeritageGuardTask0006(t *testing.T) {
	artifacts := &task0006Artifacts{}
	conservationServiceUnderTest := &Service{Artifacts: artifacts, IDs: &conservationIDs{}, Now: time.Now}
	ctx := service.WithPrincipal(context.Background(), domain.Principal{TenantID: "museum-demo", UserID: "conservator-task-0006", Role: domain.RoleConservator})
	measurements := map[string]float64{"humidity": 48}
	if _, err := conservationServiceUnderTest.RecordCondition(ctx, ConditionInput{ArtifactID: "artifact-task-0006", Summary: "stable", Severity: domain.RiskLow, Measurements: measurements, Final: true}); err != nil {
		t.Fatal(err)
	}
	measurements["humidity"] = 80
	if artifacts.saved.Measurements["humidity"] != 48 {
		t.Fatalf("condition report retained caller-owned measurements: %#v", artifacts.saved.Measurements)
	}
}
