package worker

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type task0024Store struct {
	fakeWorkerStore
	releaseContextErr error
}

func (s *task0024Store) ReleaseLease(ctx context.Context, id, owner string, now time.Time) error {
	s.releaseContextErr = ctx.Err()
	return s.fakeWorkerStore.ReleaseLease(context.Background(), id, owner, now)
}

func TestHeritageGuardTask0024(t *testing.T) {
	store := &task0024Store{fakeWorkerStore: fakeWorkerStore{jobs: []domain.WorkerJob{{ID: "job-task-0024", Kind: "cancel", Status: domain.JobPending, MaxAttempts: 3}}}}
	runner := &Runner{Store: store, Owner: "worker-task-0024", LeaseDuration: time.Second, Now: fixedNow, Logger: testLogger(), Handlers: map[string]Handler{"cancel": func(ctx context.Context, _ domain.WorkerJob) error { <-ctx.Done(); return ctx.Err() }}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.runOne(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done
	if store.releaseContextErr != nil {
		t.Fatalf("lease release must survive handler cancellation: %v", store.releaseContextErr)
	}
}
