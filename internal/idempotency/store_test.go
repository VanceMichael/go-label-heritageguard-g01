package idempotency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestNewScopeNormalizesAndHashesRequest(t *testing.T) {
	scope, err := NewScope("tenant-1", "post", "/v1/items", "key-1", []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if scope.Method != "POST" || scope.BodyHash == "" {
		t.Fatalf("scope was not normalized: %#v", scope)
	}
	if _, err := NewScope("", "POST", "/path", "key", nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected missing tenant error, got %v", err)
	}
	longKey := make([]byte, 129)
	if _, err := NewScope("tenant", "POST", "/path", string(longKey), nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected long key error, got %v", err)
	}
}

func TestIdempotencyLifecycleAgainstSQLite(t *testing.T) {
	fixture := openTestStore(t)
	store := &Store{DB: fixture.DB, Now: fixture.Now}
	scope, err := NewScope("museum-demo", "POST", "/v1/items", "same-key", []byte(`{"name":"piece"}`))
	if err != nil {
		t.Fatal(err)
	}
	record, owned, err := store.Begin(context.Background(), scope, time.Hour)
	if err != nil || !owned || record.Status != "started" {
		t.Fatalf("begin did not claim request: %#v %v %v", record, owned, err)
	}
	record, owned, err = store.Begin(context.Background(), scope, time.Hour)
	if err != nil || owned || record.Status != "started" {
		t.Fatalf("second begin should observe in-flight request: %#v %v %v", record, owned, err)
	}
	if err := store.Complete(context.Background(), scope, 201, []byte(`{"id":"piece-1"}`), "piece-1"); err != nil {
		t.Fatal(err)
	}
	record, owned, err = store.Begin(context.Background(), scope, time.Hour)
	if err != nil || owned || !record.Replayable() || record.ResourceID != "piece-1" {
		t.Fatalf("completed request was not replayable: %#v %v %v", record, owned, err)
	}
	different, err := NewScope("museum-demo", "POST", "/v1/items", "same-key", []byte(`{"name":"other"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Begin(context.Background(), different, time.Hour); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected body mismatch conflict, got %v", err)
	}
}

func TestRecordReplayable(t *testing.T) {
	if (Record{Status: "started", HTTPStatus: 201}).Replayable() {
		t.Fatal("started record cannot be replayed")
	}
	if (Record{Status: "completed", HTTPStatus: 0}).Replayable() {
		t.Fatal("statusless record cannot be replayed")
	}
	if !(Record{Status: "completed", HTTPStatus: 204}).Replayable() {
		t.Fatal("completed response should be replayable")
	}
}
