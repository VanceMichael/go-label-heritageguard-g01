package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/config"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0030(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(appLogWriter{}, nil))
	runtime, err := New(context.Background(), config.Config{Address: ":0", DatabasePath: filepath.Join(t.TempDir(), "heritageguard.db"), SessionTTL: time.Hour, WorkerInterval: 5 * time.Millisecond, ShutdownTimeout: time.Second, MaxOpenConns: 1}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	now := time.Now()
	if err := runtime.Store.EnqueueJob(context.Background(), domain.WorkerJob{ID: "job-task-0030", TenantID: "museum-demo", Kind: "shutdown-probe", AggregateID: "aggregate-task-0030", Payload: []byte(`{}`), Status: domain.JobPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	cancelReady := make(chan struct{})
	allowProbe := make(chan struct{})
	probe := make(chan error, 1)
	runtime.Worker.Handlers["shutdown-probe"] = func(ctx context.Context, _ domain.WorkerJob) error {
		close(started)
		<-ctx.Done()
		close(cancelReady)
		<-allowProbe
		probe <- runtime.Store.Ping(context.Background())
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown probe job did not start")
	}
	cancel()
	<-cancelReady
	time.Sleep(25 * time.Millisecond)
	close(allowProbe)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not shut down")
	}
	if err := <-probe; err != nil {
		t.Fatalf("database was closed before in-flight worker cleanup: %v", err)
	}
}
