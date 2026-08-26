package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/auth"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/loan"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/repository"
)

type HealthStore interface {
	Ping(context.Context) error
}

type API struct {
	Auth           *auth.Service
	Conservation   *conservation.Service
	Exhibition     *exhibition.Service
	Loans          *loan.Service
	Artifacts      repository.ArtifactRepository
	Health         HealthStore
	Idempotency    *idempotency.Store
	Middleware     Middleware
	Logger         *slog.Logger
	SensorSecret   string
	IdempotencyTTL time.Duration
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", a.live)
	mux.HandleFunc("GET /readyz", a.ready)
	mux.HandleFunc("POST /v1/session/login", a.login)
	mux.Handle("DELETE /v1/session", a.protected(http.HandlerFunc(a.logout)))
	mux.Handle("POST /v1/environment/readings", a.sensor(http.HandlerFunc(a.recordReading)))

	mux.Handle("POST /v1/users", a.mutation(http.HandlerFunc(a.createUser)))
	mux.Handle("POST /v1/users/{id}/deactivate", a.mutation(http.HandlerFunc(a.deactivateUser)))
	mux.Handle("GET /v1/artifacts", a.protected(http.HandlerFunc(a.listArtifacts)))
	mux.Handle("GET /v1/artifacts/{id}", a.protected(http.HandlerFunc(a.getArtifact)))
	mux.Handle("POST /v1/artifacts", a.mutation(http.HandlerFunc(a.registerArtifact)))
	mux.Handle("POST /v1/artifacts/{id}/conditions", a.mutation(http.HandlerFunc(a.recordCondition)))
	mux.Handle("POST /v1/artifacts/{id}/assessment/release", a.mutation(http.HandlerFunc(a.releaseAssessment)))
	mux.Handle("POST /v1/artifacts/{id}/quarantines", a.mutation(http.HandlerFunc(a.openQuarantine)))
	mux.Handle("POST /v1/treatments", a.mutation(http.HandlerFunc(a.draftTreatment)))
	mux.Handle("POST /v1/treatments/{id}/transitions", a.mutation(http.HandlerFunc(a.advanceTreatment)))
	mux.Handle("POST /v1/display-cases/{id}/reservations", a.mutation(http.HandlerFunc(a.reserveDisplayCase)))
	mux.Handle("POST /v1/display-cases/{id}/installations", a.mutation(http.HandlerFunc(a.activateInstallation)))
	mux.Handle("GET /v1/display-cases/{id}/environment-assessment", a.protected(http.HandlerFunc(a.assessEnvironment)))
	mux.Handle("POST /v1/incidents/{id}/transitions", a.mutation(http.HandlerFunc(a.advanceIncident)))
	mux.Handle("POST /v1/loans", a.mutation(http.HandlerFunc(a.createLoan)))
	mux.Handle("POST /v1/loans/{id}/submit", a.mutation(http.HandlerFunc(a.submitLoan)))
	mux.Handle("POST /v1/loans/{id}/approve", a.mutation(http.HandlerFunc(a.approveLoan)))
	mux.Handle("POST /v1/loans/{id}/custody", a.mutation(http.HandlerFunc(a.recordCustody)))

	return a.Middleware.RequestID(a.Middleware.Recover(a.Middleware.Log(mux)))
}

func (a *API) protected(next http.Handler) http.Handler {
	return a.Middleware.Authenticate(next)
}

func (a *API) mutation(next http.Handler) http.Handler {
	return a.protected(a.idempotent(next))
}

type DataResponse struct {
	Data any `json:"data"`
}

type ListResponse struct {
	Data   any `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
