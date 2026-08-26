package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Address           string
	DatabasePath      string
	SessionTTL        time.Duration
	WorkerInterval    time.Duration
	ShutdownTimeout   time.Duration
	LogLevel          string
	MaxOpenConns      int
	SensorSecret      string
	OutboxEndpoint    string
	BootstrapTenant   string
	BootstrapEmail    string
	BootstrapName     string
	BootstrapPassword string
}

func Load() (Config, error) {
	cfg := Config{
		Address:           env("HERITAGEGUARD_ADDR", ":8080"),
		DatabasePath:      env("HERITAGEGUARD_DATABASE", "heritageguard.db"),
		LogLevel:          env("HERITAGEGUARD_LOG_LEVEL", "info"),
		MaxOpenConns:      8,
		SensorSecret:      os.Getenv("HERITAGEGUARD_SENSOR_SECRET"),
		OutboxEndpoint:    os.Getenv("HERITAGEGUARD_OUTBOX_ENDPOINT"),
		BootstrapTenant:   env("HERITAGEGUARD_BOOTSTRAP_TENANT", "museum-demo"),
		BootstrapEmail:    os.Getenv("HERITAGEGUARD_BOOTSTRAP_EMAIL"),
		BootstrapName:     env("HERITAGEGUARD_BOOTSTRAP_NAME", "HeritageGuard Supervisor"),
		BootstrapPassword: os.Getenv("HERITAGEGUARD_BOOTSTRAP_PASSWORD"),
	}
	var err error
	if cfg.SessionTTL, err = duration("HERITAGEGUARD_SESSION_TTL", 8*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.WorkerInterval, err = duration("HERITAGEGUARD_WORKER_INTERVAL", 500*time.Millisecond); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration("HERITAGEGUARD_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if raw := os.Getenv("HERITAGEGUARD_MAX_OPEN_CONNS"); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 64 {
			return Config{}, fmt.Errorf("HERITAGEGUARD_MAX_OPEN_CONNS must be between 1 and 64")
		}
		cfg.MaxOpenConns = value
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("HERITAGEGUARD_DATABASE cannot be empty")
	}
	if (cfg.BootstrapEmail == "") != (cfg.BootstrapPassword == "") {
		return Config{}, fmt.Errorf("HERITAGEGUARD_BOOTSTRAP_EMAIL and HERITAGEGUARD_BOOTSTRAP_PASSWORD must be set together")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}
