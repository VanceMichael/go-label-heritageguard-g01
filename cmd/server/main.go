package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/app"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("initialize heritageguard", "error", err)
		os.Exit(1)
	}
	if err := runtime.Run(ctx); err != nil {
		logger.Error("run heritageguard", "error", err)
		os.Exit(1)
	}
}

func logLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
