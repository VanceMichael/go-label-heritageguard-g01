package sqlite

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0028(t *testing.T) {
	store := integrationStore(t, ":memory:", 1)
	fixtureArtifact(t, store, "artifact-task-0028-ready", domain.ArtifactReady)
	fixtureArtifact(t, store, "artifact-task-0028-archived", domain.ArtifactArchived)
	rows, total, err := store.ListArtifacts(context.Background(), "museum-demo", domain.Page{Limit: 10}, string(domain.ArtifactReady))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != domain.ArtifactReady || total != 1 {
		t.Fatalf("artifact list count did not use the same filter: rows=%#v total=%d", rows, total)
	}
}
