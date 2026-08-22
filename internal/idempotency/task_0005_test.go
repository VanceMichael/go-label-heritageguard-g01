package idempotency

import (
	"context"
	"testing"
	"time"
)

func TestHeritageGuardTask0005(t *testing.T) {
	fixture := openTestStore(t)
	store := &Store{DB: fixture.DB, Now: fixture.Now}
	scope, err := NewScope("museum-demo", "POST", "/v1/artifacts", "task-0005", []byte(`{"accession_no":"HG-5"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, owned, err := store.Begin(context.Background(), scope, time.Hour); err != nil || !owned {
		t.Fatalf("initial request was not claimed: owned=%v err=%v", owned, err)
	}
	if err := store.Complete(context.Background(), scope, 201, []byte(`{"id":"artifact-5"}`), "artifact-5"); err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	replay, owned, err := store.Begin(context.Background(), scope, time.Hour)
	if err != nil || owned || !replay.Replayable() || replay.ResourceID != "artifact-5" {
		t.Fatalf("completed idempotency result was lost by failure cleanup: record=%#v owned=%v err=%v", replay, owned, err)
	}
}
