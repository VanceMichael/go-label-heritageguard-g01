package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

func (a *API) idempotent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := service.PrincipalFrom(r.Context())
		if err != nil {
			writeError(r.Context(), a.Logger, w, err)
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			writeError(r.Context(), a.Logger, w, domain.FieldError{Field: "idempotency_key", Message: "Idempotency-Key header is required"})
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody+1))
		if err != nil {
			writeError(r.Context(), a.Logger, w, domain.FieldError{Field: "body", Message: "cannot read request body"})
			return
		}
		if len(body) > maxJSONBody {
			writeError(r.Context(), a.Logger, w, domain.FieldError{Field: "body", Message: "request body exceeds one MiB"})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		scope, err := idempotency.NewScope(principal.TenantID, r.Method, r.URL.Path, key, body)
		if err != nil {
			writeError(r.Context(), a.Logger, w, err)
			return
		}
		ttl := a.IdempotencyTTL
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		record, owned, err := a.Idempotency.Begin(r.Context(), scope, ttl)
		if err != nil {
			writeError(r.Context(), a.Logger, w, err)
			return
		}
		if !owned {
			if record.Replayable() {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Idempotency-Replayed", "true")
				w.WriteHeader(record.HTTPStatus)
				_, _ = w.Write(record.Body)
				return
			}
			writeError(r.Context(), a.Logger, w, fmt.Errorf("idempotent request is still in progress: %w", domain.ErrConflict))
			return
		}

		buffer := newBufferedResponse()
		next.ServeHTTP(buffer, r)
		if buffer.status >= http.StatusInternalServerError {
			if forgetErr := a.Idempotency.Forget(r.Context(), scope); forgetErr != nil {
				a.Logger.Error("forget failed idempotent request", "request_id", service.RequestIDFrom(r.Context()), "error", forgetErr)
			}
			buffer.commit(w)
			return
		}
		if err := a.Idempotency.Complete(r.Context(), scope, buffer.status, buffer.body.Bytes(), resourceID(buffer.header)); err != nil {
			a.Logger.Error("complete idempotent request", "request_id", service.RequestIDFrom(r.Context()), "error", err)
			writeError(r.Context(), a.Logger, w, fmt.Errorf("persist idempotent response: %w", err))
			return
		}
		buffer.commit(w)
	})
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header), status: http.StatusOK}
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if status < 100 || status > 999 {
		panic("invalid HTTP status")
	}
	if b.body.Len() != 0 || b.status != http.StatusOK {
		return
	}
	b.status = status
}

func (b *bufferedResponse) Write(body []byte) (int, error) {
	return b.body.Write(body)
}

func (b *bufferedResponse) commit(w http.ResponseWriter) {
	for key, values := range b.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}

func resourceID(header http.Header) string {
	return strings.TrimSpace(header.Get("X-Resource-ID"))
}
