package exhibition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

type task0010Cases struct{}

func (task0010Cases) GetDisplayCase(context.Context, string, string) (domain.DisplayCase, error) { return domain.DisplayCase{}, nil }
func (task0010Cases) ReserveDisplayCase(context.Context, domain.DisplayCase, int64) error { return nil }
func (task0010Cases) ActivateInstallation(context.Context, domain.Installation, domain.Artifact, domain.DisplayCase, int64, int64) error { return nil }
func (task0010Cases) SaveReadingAndEnqueue(context.Context, domain.EnvironmentReading, domain.WorkerJob) error { return nil }
func (task0010Cases) AssessEnvironment(ctx context.Context, _ string, _ string, _ time.Time, _ time.Time) (domain.EnvironmentAssessment, error) {
	if err := ctx.Err(); err != nil {
		return domain.EnvironmentAssessment{}, err
	}
	return domain.EnvironmentAssessment{Ready: true, ReadingCount: 1}, nil
}
func (task0010Cases) OpenIncidentAndOutbox(context.Context, domain.Incident, domain.OutboxEvent) (domain.Incident, bool, error) { return domain.Incident{}, false, nil }
func (task0010Cases) GetIncident(context.Context, string, string) (domain.Incident, error) { return domain.Incident{}, nil }
func (task0010Cases) UpdateIncident(context.Context, domain.Incident, int64) error { return nil }

func TestHeritageGuardTask0010(t *testing.T) {
	ctx := service.WithPrincipal(context.Background(), domain.Principal{TenantID: "museum-demo", UserID: "coordinator", Role: domain.RoleCoordinator})
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	service := &Service{Cases: task0010Cases{}, Now: time.Now}
	_, err := service.Assess(ctx, "case-east-01", 15*time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled assessment must not produce a readiness decision, got %v", err)
	}
}
