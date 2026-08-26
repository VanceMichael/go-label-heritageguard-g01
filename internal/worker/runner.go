package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
)

type Handler func(context.Context, domain.WorkerJob) error

type Runner struct {
	Store         repository.WorkerRepository
	Owner         string
	PollInterval  time.Duration
	LeaseDuration time.Duration
	Now           func() time.Time
	Logger        *slog.Logger
	Handlers      map[string]Handler

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func (r *Runner) Run(ctx context.Context) error {
	if r.Store == nil || r.Owner == "" || r.Now == nil {
		return fmt.Errorf("worker runner is not configured")
	}
	if r.PollInterval <= 0 || r.LeaseDuration <= 0 {
		return fmt.Errorf("worker intervals must be positive")
	}
	if r.running == nil {
		r.running = make(map[string]context.CancelFunc)
	}
	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.cancelRunning()
			return ctx.Err()
		case <-ticker.C:
			if err := r.runOne(ctx); err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, context.Canceled) {
				r.Logger.Error("worker iteration failed", "error", err)
			}
		}
	}
}

func (r *Runner) runOne(ctx context.Context) error {
	job, err := r.Store.ClaimJob(ctx, r.Owner, r.Now(), r.LeaseDuration)
	if err != nil {
		return err
	}
	handler := r.Handlers[job.Kind]
	if handler == nil {
		return r.Store.FailJob(ctx, job.ID, r.Owner, r.Now(), fmt.Errorf("no handler for job kind %q", job.Kind))
	}
	jobCtx, cancel := context.WithCancel(ctx)
	r.track(job.ID, cancel)
	defer func() {
		cancel()
		r.untrack(job.ID)
	}()
	result := handler(jobCtx, job)
	if result == nil {
		return r.Store.CompleteJob(ctx, job.ID, r.Owner, r.Now())
	}
	if errors.Is(result, context.Canceled) || errors.Is(result, context.DeadlineExceeded) {
		releaseDeadline := r.Now().Add(2 * time.Second)
		releaseCtx, releaseCancel := context.WithDeadline(ctx, releaseDeadline)
		defer releaseCancel()
		return errors.Join(result, r.Store.ReleaseLease(releaseCtx, job.ID, r.Owner, r.Now()))
	}
	if job.Attempts >= job.MaxAttempts {
		return r.Store.FailJob(ctx, job.ID, r.Owner, r.Now(), result)
	}
	availableAt := r.Now().Add(retryBackoff(job.Attempts))
	return r.Store.RetryJob(ctx, job.ID, r.Owner, r.Now(), availableAt, result)
}

func (r *Runner) track(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	if r.running == nil {
		r.running = make(map[string]context.CancelFunc)
	}
	r.running[id] = cancel
	r.mu.Unlock()
}

func (r *Runner) untrack(id string) {
	r.mu.Lock()
	delete(r.running, id)
	r.mu.Unlock()
}

func (r *Runner) cancelRunning() {
	r.mu.Lock()
	cancellations := make([]context.CancelFunc, 0, len(r.running))
	for _, cancel := range r.running {
		cancellations = append(cancellations, cancel)
	}
	r.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}
