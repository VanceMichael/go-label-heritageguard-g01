package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

)

func TestHeritageGuardTask0029(t *testing.T) {
	middleware := Middleware{IDs: &apiIDs{}, Logger: slog.New(slog.NewTextHandler(apiLogWriter{}, nil))}
	panicHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("rendering failure") })
	handler := middleware.RequestID(middleware.Recover(panicHandler))
	request := httptest.NewRequest(http.MethodGet, "/v1/panic", nil)
	request.Header.Set("X-Request-ID", "task-0029-request")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body ErrorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.RequestID != "task-0029-request" {
		t.Fatalf("panic response lost the request correlation id: %#v", body)
	}
}
