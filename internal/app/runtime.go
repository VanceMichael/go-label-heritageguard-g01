package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/auth"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/bootstrap"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/config"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/eventbus"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/httpapi"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/loan"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/outbox"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/worker"
)

type Runtime struct {
	Config     config.Config
	Logger     *slog.Logger
	Store      *sqlite.Store
	Events     *eventbus.Bus
	Server     *http.Server
	Worker     *worker.Runner
	Dispatcher *outbox.Dispatcher
	closeOnce  sync.Once
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	store, err := sqlite.Open(ctx, cfg.DatabasePath, cfg.MaxOpenConns, logger)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Runtime, error) {
		return nil, errors.Join(err, store.Close())
	}
	ids := service.RandomIDs{}
	created, err := bootstrap.EnsureSupervisor(ctx, store, ids, time.Now, bootstrap.SupervisorConfig{
		TenantID: cfg.BootstrapTenant,
		Email:    cfg.BootstrapEmail,
		Name:     cfg.BootstrapName,
		Password: cfg.BootstrapPassword,
	})
	if err != nil {
		return fail(err)
	}
	if created {
		logger.Info("bootstrap supervisor created", "tenant_id", cfg.BootstrapTenant, "email", cfg.BootstrapEmail)
	}
	authService := &auth.Service{
		Users: store, Sessions: store, Deactivator: store,
		IDs: ids, Tokens: service.RandomTokens{}, SessionTTL: cfg.SessionTTL, Now: time.Now,
	}
	conservationService := &conservation.Service{Artifacts: store, Cases: store, IDs: ids, Now: time.Now}
	events := eventbus.New()
	exhibitionService := &exhibition.Service{Artifacts: store, Cases: store, Events: events, IDs: ids, Now: time.Now}
	loanService := &loan.Service{Artifacts: store, Loans: store, Approver: store, IDs: ids, Now: time.Now}
	middleware := httpapi.Middleware{Auth: authService, IDs: ids, Logger: logger}
	api := &httpapi.API{
		Auth: authService, Conservation: conservationService, Exhibition: exhibitionService,
		Loans: loanService, Artifacts: store, Health: store,
		Idempotency: &idempotency.Store{DB: store.DB, Now: time.Now},
		Middleware:  middleware, Logger: logger, SensorSecret: cfg.SensorSecret,
		IdempotencyTTL: 24 * time.Hour,
	}
	runner := &worker.Runner{
		Store: store, Owner: ids.New("worker"), PollInterval: cfg.WorkerInterval,
		LeaseDuration: 30 * time.Second, Now: time.Now, Logger: logger,
		Handlers: map[string]worker.Handler{
			exhibition.EnvironmentAssessmentJob: exhibitionService.ProcessAssessmentJob,
		},
	}
	runtime := &Runtime{
		Config: cfg,
		Logger: logger,
		Store:  store,
		Events: events,
		Worker: runner,
		Server: &http.Server{
			Addr:              cfg.Address,
			Handler:           api.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
	if cfg.OutboxEndpoint != "" {
		runtime.Dispatcher = &outbox.Dispatcher{
			Store: store,
			Sender: outbox.HTTPSender{
				Client:   &http.Client{Timeout: 10 * time.Second},
				Endpoint: cfg.OutboxEndpoint,
			},
			Owner: ids.New("outbox"), PollInterval: cfg.WorkerInterval,
			LeaseDuration: 30 * time.Second, Now: time.Now, Logger: logger,
		}
	}
	return runtime, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", r.Server.Addr)
	if err != nil {
		_ = r.Close()
		return fmt.Errorf("listen on %s: %w", r.Server.Addr, err)
	}
	r.Logger.Info("heritageguard started", "address", listener.Addr().String(), "database", r.Config.DatabasePath)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	components := 2
	if r.Dispatcher != nil {
		components++
	}
	errCh := make(chan error, components)
	var wg sync.WaitGroup
	start := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("%s: %w", name, err)
			}
		}()
	}
	start("HTTP server", func() error { return r.Server.Serve(listener) })
	start("worker", func() error { return r.Worker.Run(runCtx) })
	if r.Dispatcher != nil {
		start("outbox dispatcher", func() error { return r.Dispatcher.Run(runCtx) })
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
		r.Logger.Error("runtime component stopped", "error", runErr)
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), r.Config.ShutdownTimeout)
	defer shutdownCancel()
	shutdownErr := r.Server.Shutdown(shutdownCtx)
	closeErr := r.Close()
	wg.Wait()
	r.Logger.Info("heritageguard stopped")
	return errors.Join(runErr, shutdownErr, closeErr)
}

func (r *Runtime) Close() error {
	var err error
	r.closeOnce.Do(func() {
		if r.Events != nil {
			r.Events.Close()
		}
		err = r.Store.Close()
	})
	return err
}
