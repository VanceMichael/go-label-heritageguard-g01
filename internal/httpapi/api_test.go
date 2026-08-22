package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/auth"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/loan"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
)

type apiLogWriter struct{}

func (apiLogWriter) Write(p []byte) (int, error) { return len(p), nil }

type apiIDs struct{ index int }

func (g *apiIDs) New(prefix string) string {
	g.index++
	return prefix + "-api-" + string(rune('a'+g.index))
}

type apiTokens struct{ index int }

func (g *apiTokens) NewToken() (string, error) {
	g.index++
	return "api-token-" + string(rune('a'+g.index)), nil
}

type apiFixture struct {
	store      *sqlite.Store
	handler    http.Handler
	supervisor domain.User
	password   string
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(apiLogWriter{}, nil))
	store, err := sqlite.Open(context.Background(), ":memory:", 1, logger)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	password := "a-valid-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{ID: "supervisor", TenantID: "museum-demo", Email: "supervisor@museum.invalid", DisplayName: "Supervisor",
		Role: domain.RoleSupervisor, PasswordHash: hash, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	ids := &apiIDs{}
	authService := &auth.Service{Users: store, Sessions: store, Deactivator: store, IDs: ids, Tokens: &apiTokens{}, SessionTTL: time.Hour, Now: store.Now}
	conservationService := &conservation.Service{Artifacts: store, Cases: store, IDs: ids, Now: store.Now}
	exhibitionService := &exhibition.Service{Artifacts: store, Cases: store, IDs: ids, Now: store.Now}
	loanService := &loan.Service{Artifacts: store, Loans: store, Approver: store, IDs: ids, Now: store.Now}
	logger = slog.New(slog.NewTextHandler(apiLogWriter{}, nil))
	api := &API{Auth: authService, Conservation: conservationService, Exhibition: exhibitionService, Loans: loanService,
		Artifacts: store, Health: store, Idempotency: &idempotency.Store{DB: store.DB, Now: store.Now},
		Middleware: Middleware{Auth: authService, IDs: ids, Logger: logger}, Logger: logger, SensorSecret: "sensor-secret", IdempotencyTTL: time.Hour}
	return &apiFixture{store: store, handler: api.Handler(), supervisor: user, password: password}
}

func apiRequest(t *testing.T, fixture *apiFixture, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	var err error
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func apiBody(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func apiLogin(t *testing.T, fixture *apiFixture) string {
	t.Helper()
	response := apiRequest(t, fixture, http.MethodPost, "/v1/session/login", auth.LoginInput{TenantID: fixture.supervisor.TenantID, Email: fixture.supervisor.Email, Password: fixture.password}, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status %d body=%#v", response.StatusCode, apiBody(t, response))
	}
	body := apiBody(t, response)
	data, ok := body["data"].(map[string]any)
	if !ok || data["token"] == "" {
		t.Fatalf("login response missing token: %#v", body)
	}
	return data["token"].(string)
}

func TestHealthAndReadinessEndpoints(t *testing.T) {
	fixture := newAPIFixture(t)
	response := apiRequest(t, fixture, http.MethodGet, "/livez", nil, nil)
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Request-ID") == "" {
		t.Fatalf("live endpoint contract wrong: status=%d request_id=%q", response.StatusCode, response.Header.Get("X-Request-ID"))
	}
	response = apiRequest(t, fixture, http.MethodGet, "/readyz", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ready endpoint should be healthy: %d %#v", response.StatusCode, apiBody(t, response))
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	response = apiRequest(t, fixture, http.MethodGet, "/readyz", nil, nil)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("closed database should fail readiness: %d %#v", response.StatusCode, apiBody(t, response))
	}
}

func TestLoginLogoutAndAuthenticationMapping(t *testing.T) {
	fixture := newAPIFixture(t)
	response := apiRequest(t, fixture, http.MethodGet, "/v1/artifacts", nil, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing bearer should be unauthorized, got %d", response.StatusCode)
	}
	body := apiBody(t, response)
	if body["error"].(map[string]any)["code"] != "unauthorized" {
		t.Fatalf("unexpected auth error: %#v", body)
	}
	token := apiLogin(t, fixture)
	response = apiRequest(t, fixture, http.MethodDelete, "/v1/session", nil, map[string]string{"Authorization": "Bearer " + token})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status wrong: %d", response.StatusCode)
	}
	response = apiRequest(t, fixture, http.MethodGet, "/v1/artifacts", nil, map[string]string{"Authorization": "Bearer " + token})
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token should be unauthorized, got %d", response.StatusCode)
	}
}

func TestRegisterArtifactRequiresIdempotencyAndReplaysResponse(t *testing.T) {
	fixture := newAPIFixture(t)
	token := apiLogin(t, fixture)
	body := conservation.RegisterArtifactInput{AccessionNo: "HTTP-001", Name: "Exhibition brick", Material: "clay", Period: "Western Xia", RiskClass: domain.RiskLow,
		InitialZoneID: "zone-general", Summary: "stable intake", Measurements: map[string]float64{"humidity": 48}}
	headers := map[string]string{"Authorization": "Bearer " + token}
	response := apiRequest(t, fixture, http.MethodPost, "/v1/artifacts", body, headers)
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing idempotency key should fail: %d %#v", response.StatusCode, apiBody(t, response))
	}
	headers["Idempotency-Key"] = "artifact-http-1"
	response = apiRequest(t, fixture, http.MethodPost, "/v1/artifacts", body, headers)
	if response.StatusCode != http.StatusCreated || response.Header.Get("X-Resource-ID") == "" {
		t.Fatalf("artifact creation status wrong: %d %#v", response.StatusCode, apiBody(t, response))
	}
	first := apiBody(t, response)
	resourceID := response.Header.Get("X-Resource-ID")
	response = apiRequest(t, fixture, http.MethodPost, "/v1/artifacts", body, headers)
	if response.StatusCode != http.StatusCreated || response.Header.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay contract wrong: status=%d replay=%q", response.StatusCode, response.Header.Get("Idempotency-Replayed"))
	}
	second := apiBody(t, response)
	if first["data"].(map[string]any)["artifact"].(map[string]any)["id"] != second["data"].(map[string]any)["artifact"].(map[string]any)["id"] {
		t.Fatalf("replay returned a different resource: first=%#v second=%#v", first, second)
	}
	var count int
	if err := fixture.store.DB.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE id = ?`, resourceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent request created %d resources", count)
	}
}

func TestIdempotencyKeyCannotBeReusedWithDifferentBody(t *testing.T) {
	fixture := newAPIFixture(t)
	token := apiLogin(t, fixture)
	headers := map[string]string{"Authorization": "Bearer " + token, "Idempotency-Key": "same-body-key"}
	first := conservation.RegisterArtifactInput{AccessionNo: "HTTP-002", Name: "First", Material: "paper", Period: "Qing", RiskClass: domain.RiskLow, InitialZoneID: "zone-general", Summary: "stable"}
	response := apiRequest(t, fixture, http.MethodPost, "/v1/artifacts", first, headers)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("first idempotent write failed: %d %#v", response.StatusCode, apiBody(t, response))
	}
	second := first
	second.Name = "Different"
	response = apiRequest(t, fixture, http.MethodPost, "/v1/artifacts", second, headers)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("body mismatch should be conflict: %d %#v", response.StatusCode, apiBody(t, response))
	}
}

func TestSensorEndpointRequiresConstantTimeSecretAndTenant(t *testing.T) {
	fixture := newAPIFixture(t)
	reading := exhibition.ReadingInput{DisplayCaseID: "case-east-01", DeviceID: "device-http", Sequence: 1, TemperatureC: 20, Humidity: 50, ObservedAt: fixture.store.Now()}
	response := apiRequest(t, fixture, http.MethodPost, "/v1/environment/readings", reading, map[string]string{"X-Sensor-Secret": "wrong", "X-Tenant-ID": "museum-demo"})
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong sensor secret should be unauthorized, got %d", response.StatusCode)
	}
	response = apiRequest(t, fixture, http.MethodPost, "/v1/environment/readings", reading, map[string]string{"X-Sensor-Secret": "sensor-secret"})
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing tenant should be invalid, got %d", response.StatusCode)
	}
	response = apiRequest(t, fixture, http.MethodPost, "/v1/environment/readings", reading, map[string]string{"X-Sensor-Secret": "sensor-secret", "X-Tenant-ID": "museum-demo"})
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("valid sensor reading should be accepted, got %d %#v", response.StatusCode, apiBody(t, response))
	}
}

func TestJSONDecoderRejectsUnknownAndTrailingValues(t *testing.T) {
	fixture := newAPIFixture(t)
	token := apiLogin(t, fixture)
	request := httptest.NewRequest(http.MethodPost, "/v1/artifacts", bytes.NewBufferString(`{"unknown":true}`))
	var err error
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", "decode-unknown")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown fields should fail: %d", response.StatusCode)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/artifacts", bytes.NewBufferString(`{"accession_no":"x"} {"extra":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", "decode-trailing")
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	fixture.handler.ServeHTTP(recorder, request)
	response = recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("trailing JSON should fail: %d", response.StatusCode)
	}
}

func TestErrorClassificationPreservesWrappedRuntimeCauses(t *testing.T) {
	if status, code, _ := classifyError(errors.Join(context.Canceled, errors.New("transport"))); status != 499 || code != "request_cancelled" {
		t.Fatalf("cancelled error classification wrong: %d %s", status, code)
	}
	if status, code, _ := classifyError(domain.DependencyError{Operation: "outbox", Err: errors.New("timeout")}); status != http.StatusServiceUnavailable || code != "dependency_unavailable" {
		t.Fatalf("dependency classification wrong: %d %s", status, code)
	}
	if status, code, _ := classifyError(domain.FieldError{Field: "name", Message: "required"}); status != http.StatusUnprocessableEntity || code != "invalid_input" {
		t.Fatalf("field classification wrong: %d %s", status, code)
	}
}
