package config

import (
	"testing"
	"time"
)

func TestLoadUsesSafeDefaultsAndConfiguredDurations(t *testing.T) {
	for _, key := range []string{
		"HERITAGEGUARD_ADDR", "HERITAGEGUARD_DATABASE", "HERITAGEGUARD_SESSION_TTL", "HERITAGEGUARD_WORKER_INTERVAL",
		"HERITAGEGUARD_SHUTDOWN_TIMEOUT", "HERITAGEGUARD_LOG_LEVEL", "HERITAGEGUARD_MAX_OPEN_CONNS",
		"HERITAGEGUARD_SENSOR_SECRET", "HERITAGEGUARD_OUTBOX_ENDPOINT", "HERITAGEGUARD_BOOTSTRAP_TENANT",
		"HERITAGEGUARD_BOOTSTRAP_EMAIL", "HERITAGEGUARD_BOOTSTRAP_NAME", "HERITAGEGUARD_BOOTSTRAP_PASSWORD",
	} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != ":8080" || cfg.DatabasePath != "heritageguard.db" || cfg.SessionTTL != 8*time.Hour || cfg.WorkerInterval != 500*time.Millisecond || cfg.ShutdownTimeout != 10*time.Second || cfg.MaxOpenConns != 8 {
		t.Fatalf("default configuration wrong: %#v", cfg)
	}
	t.Setenv("HERITAGEGUARD_ADDR", "127.0.0.1:9090")
	t.Setenv("HERITAGEGUARD_DATABASE", "/tmp/heritageguard.db")
	t.Setenv("HERITAGEGUARD_SESSION_TTL", "2h")
	t.Setenv("HERITAGEGUARD_WORKER_INTERVAL", "250ms")
	t.Setenv("HERITAGEGUARD_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("HERITAGEGUARD_MAX_OPEN_CONNS", "12")
	t.Setenv("HERITAGEGUARD_SENSOR_SECRET", "secret")
	t.Setenv("HERITAGEGUARD_BOOTSTRAP_EMAIL", "supervisor@museum.invalid")
	t.Setenv("HERITAGEGUARD_BOOTSTRAP_PASSWORD", "a-valid-password")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:9090" || cfg.SessionTTL != 2*time.Hour || cfg.WorkerInterval != 250*time.Millisecond || cfg.ShutdownTimeout != 3*time.Second || cfg.MaxOpenConns != 12 || cfg.SensorSecret != "secret" {
		t.Fatalf("configured values not loaded: %#v", cfg)
	}
}

func TestLoadRejectsInvalidDurationsLimitsAndBootstrapPair(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"session duration", "HERITAGEGUARD_SESSION_TTL", "0s"},
		{"worker duration", "HERITAGEGUARD_WORKER_INTERVAL", "not-a-duration"},
		{"shutdown duration", "HERITAGEGUARD_SHUTDOWN_TIMEOUT", "-1s"},
		{"max connections", "HERITAGEGUARD_MAX_OPEN_CONNS", "65"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
	t.Setenv("HERITAGEGUARD_BOOTSTRAP_EMAIL", "supervisor@museum.invalid")
	t.Setenv("HERITAGEGUARD_BOOTSTRAP_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("partial bootstrap credentials were accepted")
	}
	t.Setenv("HERITAGEGUARD_BOOTSTRAP_EMAIL", "")
	t.Setenv("HERITAGEGUARD_BOOTSTRAP_PASSWORD", "a-valid-password")
	if _, err := Load(); err == nil {
		t.Fatal("password without bootstrap email was accepted")
	}
}
