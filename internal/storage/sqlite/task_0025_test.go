package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0025(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	now := store.Now()
	event := domain.OutboxEvent{ID: "outbox-task-0025", TenantID: "museum-demo", Topic: "incident.opened", AggregateID: "incident-task-0025", IdempotencyKey: "task-0025", Payload: []byte(`{}`), Status: domain.JobPending, MaxAttempts: 1, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.EnqueueOutbox(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimOutbox(context.Background(), "dispatcher-task-0025", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("expected first attempt, got %d", claimed.Attempts)
	}
	if err := store.FinishOutbox(context.Background(), claimed.ID, "dispatcher-task-0025", now, errors.New("remote unavailable")); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := store.DB.QueryRow(`SELECT status FROM outbox_events WHERE id = ?`, claimed.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.JobFailed) {
		t.Fatalf("max-attempt outbox event was not terminal: %s", status)
	}
}
