package outbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0020(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()
	sender := HTTPSender{Client: server.Client(), Endpoint: server.URL}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- sender.Send(ctx, domain.OutboxEvent{ID: "event-task-0020", Topic: "loan.delivery", Payload: []byte(`{}`)}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("delivery request did not reach the carrier")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled delivery must remain an unknown failure, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled delivery did not return")
	}
}
