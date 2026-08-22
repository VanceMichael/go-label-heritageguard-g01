package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

type ErrorBody struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, err error) {
	status, code, message := classifyError(err)
	requestID := service.RequestIDFrom(ctx)
	if status >= 500 {
		logger.Error("request failed", "request_id", requestID, "error", err)
	}
	writeJSON(w, status, ErrorBody{Error: APIError{Code: code, Message: message, RequestID: requestID}})
}

func classifyError(err error) (int, string, string) {
	switch {
	case errors.Is(err, context.Canceled):
		return 499, "request_cancelled", "request was cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "deadline_exceeded", "operation exceeded its deadline"
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrExpired), errors.Is(err, domain.ErrRevoked):
		return http.StatusUnauthorized, "unauthorized", "authentication is required or no longer valid"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden", "the authenticated role cannot perform this operation"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found", "the requested resource was not found"
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusUnprocessableEntity, "invalid_input", err.Error()
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrVersion), errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict, "conflict", "the operation conflicts with current state"
	case errors.Is(err, domain.ErrIllegalState), errors.Is(err, domain.ErrPrecondition), errors.Is(err, domain.ErrCapacity):
		return http.StatusUnprocessableEntity, "precondition_failed", err.Error()
	case errors.Is(err, domain.ErrUnavailable):
		return http.StatusServiceUnavailable, "dependency_unavailable", "a required dependency is unavailable"
	default:
		return http.StatusInternalServerError, "internal_error", "an internal error occurred"
	}
}
