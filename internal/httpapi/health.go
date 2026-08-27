package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (a *API) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.Health.Ping(ctx); err != nil {
		writeError(r.Context(), a.Logger, w, domain.DependencyError{Operation: "readiness database ping", Err: err})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) sensor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := []byte(strings.TrimSpace(r.Header.Get("X-Sensor-Secret")))
		expected := []byte(a.SensorSecret)
		if len(expected) == 0 || len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
			writeError(r.Context(), a.Logger, w, domain.ErrUnauthorized)
			return
		}
		if strings.TrimSpace(r.Header.Get("X-Tenant-ID")) == "" {
			writeError(r.Context(), a.Logger, w, domain.FieldError{Field: "tenant_id", Message: "X-Tenant-ID header is required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
