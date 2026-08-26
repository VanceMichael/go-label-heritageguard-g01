package idempotency

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/storage/sqlite"
)

type testStore struct {
	*sqlite.Store
	Now func() time.Time
}

func openTestStore(t *testing.T) *testStore {
	t.Helper()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	store, err := sqlite.Open(context.Background(), ":memory:", 1, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return now }
	t.Cleanup(func() { _ = store.Close() })
	return &testStore{Store: store, Now: func() time.Time { return now }}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
