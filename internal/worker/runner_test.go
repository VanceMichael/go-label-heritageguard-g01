package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type fakeWorkerStore struct {
	mu        sync.Mutex
	jobs      []domain.WorkerJob
	claimed   []string
	completed []string
	retried   []string
	released  []string
	failed    []string
}

func (s *fakeWorkerStore) ClaimJob(_ context.Context, owner string, now time.Time, lease time.Duration) (domain.WorkerJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.jobs {
		job := s.jobs[index]
		if job.Status != domain.JobPending && job.Status != domain.JobRetry {
			continue
		}
		job.Status = domain.JobRunning
		job.Attempts++
		expires := now.Add(lease)
		job.LeaseOwner = owner
		job.LeaseExpiresAt = &expires
		s.jobs[index] = job
		s.claimed = append(s.claimed, job.ID)
		return job, nil
	}
	return domain.WorkerJob{}, domain.ErrNotFound
}

func (s *fakeWorkerStore) finish(id string, target domain.JobStatus, bucket *[]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.jobs {
		if s.jobs[index].ID == id {
			s.jobs[index].Status = target
			*bucket = append(*bucket, id)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (s *fakeWorkerStore) CompleteJob(_ context.Context, id, _ string, _ time.Time) error {
	return s.finish(id, domain.JobSucceeded, &s.completed)
}

func (s *fakeWorkerStore) RetryJob(_ context.Context, id, _ string, _, _ time.Time, _ error) error {
	return s.finish(id, domain.JobRetry, &s.retried)
}

func (s *fakeWorkerStore) FailJob(_ context.Context, id, _ string, _ time.Time, _ error) error {
	return s.finish(id, domain.JobFailed, &s.failed)
}

func (s *fakeWorkerStore) ReleaseLease(_ context.Context, id, _ string, _ time.Time) error {
	return s.finish(id, domain.JobRetry, &s.released)
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(testLogWriter{}, nil)) }

type testLogWriter struct{}

func (testLogWriter) Write(p []byte) (int, error) { return len(p), nil }

func fixedNow() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) }

func TestRunnerCompletesSuccessfulJob(t *testing.T) {
	store := &fakeWorkerStore{jobs: []domain.WorkerJob{{ID: "job-1", Kind: "ok", Status: domain.JobPending, MaxAttempts: 3}}}
	runner := &Runner{
		Store: store, Owner: "worker-1", PollInterval: time.Millisecond, LeaseDuration: time.Second,
		Now: fixedNow, Logger: testLogger(), Handlers: map[string]Handler{"ok": func(context.Context, domain.WorkerJob) error { return nil }},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	select {
	case <-time.After(100 * time.Millisecond):
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not run")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected worker cancellation, got %v", err)
	}
	if len(store.completed) != 1 {
		t.Fatalf("expected one completed job, got %#v", store.completed)
	}
}

func TestRunnerRetriesThenFails(t *testing.T) {
	store := &fakeWorkerStore{jobs: []domain.WorkerJob{{ID: "job-2", Kind: "flaky", Status: domain.JobPending, MaxAttempts: 2}}}
	runner := &Runner{
		Store: store, Owner: "worker-2", PollInterval: time.Millisecond, LeaseDuration: time.Second,
		Now: fixedNow, Logger: testLogger(), Handlers: map[string]Handler{"flaky": func(context.Context, domain.WorkerJob) error { return errors.New("temporary") }},
	}
	if err := runner.runOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.retried) != 1 || len(store.failed) != 0 {
		t.Fatalf("expected retry on first attempt: %#v %#v", store.retried, store.failed)
	}
	if err := runner.runOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.failed) != 1 {
		t.Fatalf("expected failure after max attempts: %#v", store.failed)
	}
}

func TestRunnerReleasesLeaseOnCancellation(t *testing.T) {
	store := &fakeWorkerStore{jobs: []domain.WorkerJob{{ID: "job-3", Kind: "cancel", Status: domain.JobPending, MaxAttempts: 3}}}
	runner := &Runner{
		Store: store, Owner: "worker-3", PollInterval: time.Millisecond, LeaseDuration: time.Second,
		Now: fixedNow, Logger: testLogger(), Handlers: map[string]Handler{"cancel": func(ctx context.Context, _ domain.WorkerJob) error { <-ctx.Done(); return ctx.Err() }},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.runOne(ctx) }()
	time.Sleep(5 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if len(store.released) != 1 {
		t.Fatalf("expected lease release, got %#v", store.released)
	}
}

func TestRunnerConfigurationValidation(t *testing.T) {
	runner := Runner{}
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("unconfigured worker should fail")
	}
}
