package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0023(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	now := store.Now()
	job := domain.WorkerJob{ID: "job-task-0023", TenantID: "museum-demo", Kind: "environment.assess", AggregateID: "case-east-01", Payload: []byte(`{}`), Status: domain.JobPending, MaxAttempts: 5, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.EnqueueJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimJob(context.Background(), "worker-a", now, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimJob(context.Background(), "worker-b", now, time.Hour); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a live lease must not be claimable by another worker, got %v", err)
	}
}
