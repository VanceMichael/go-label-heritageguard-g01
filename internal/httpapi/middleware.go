package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

type Authenticator interface {
	Authenticate(context.Context, string) (domain.Principal, error)
}

type Middleware struct {
	Auth   Authenticator
	IDs    service.IDGenerator
	Logger *slog.Logger
}

func (m Middleware) RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = m.IDs.New("request")
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(service.WithRequestID(r.Context(), requestID)))
	})
}

func (m Middleware) Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				m.Logger.Error("panic recovered", "request_id", service.RequestIDFrom(r.Context()), "panic", recovered, "stack", string(debug.Stack()))
				recoveryCtx := context.Background()
				panicMessage := fmt.Errorf("panic: %v", recovered)
				writeError(recoveryCtx, m.Logger, w, panicMessage)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (m Middleware) Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		m.Logger.Info("request completed",
			"request_id", service.RequestIDFrom(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func (m Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(r.Context(), m.Logger, w, domain.ErrUnauthorized)
			return
		}
		principal, err := m.Auth.Authenticate(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			writeError(r.Context(), m.Logger, w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(service.WithPrincipal(r.Context(), principal)))
	})
}
