package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/config"
)

type appLogWriter struct{}

func (appLogWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestNewAssemblesRecoverableRuntime(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(appLogWriter{}, nil))
	cfg := config.Config{
		Address: ":0", DatabasePath: filepath.Join(t.TempDir(), "heritageguard.db"), SessionTTL: time.Hour,
		WorkerInterval: 10 * time.Millisecond, ShutdownTimeout: time.Second, MaxOpenConns: 1,
		BootstrapTenant: "museum-demo", BootstrapEmail: "supervisor@museum.invalid", BootstrapName: "Supervisor", BootstrapPassword: "a-valid-password",
		SensorSecret: "sensor-secret",
	}
	runtime, err := New(context.Background(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Server == nil || runtime.Server.Handler == nil || runtime.Worker == nil || runtime.Store == nil {
		t.Fatal("runtime was not fully assembled")
	}
	user, err := runtime.Store.FindUser(context.Background(), "museum-demo", "")
	if err == nil || user.ID != "" {
		t.Fatal("empty user lookup unexpectedly succeeded")
	}
	var users int
	if err := runtime.Store.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE tenant_id = 'museum-demo' AND role = 'supervisor'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("runtime did not bootstrap supervisor: %d", users)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal("close must be idempotent: ", err)
	}
}
