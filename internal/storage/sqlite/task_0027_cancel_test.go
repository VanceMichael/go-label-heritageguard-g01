package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestHeritageGuardTask0027(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Migrate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled migration must stop before applying schema changes, got %v", err)
	}
}
