package exhibition

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestConcurrentReservationOfSameCaseHasSingleOwner(t *testing.T) {
	store := exhibitionStore(t)
	coordinatorA := exhibitionUser(t, store, "coordinator-a", domain.RoleCoordinator)
	coordinatorB := exhibitionUser(t, store, "coordinator-b", domain.RoleCoordinator)
	artifactA := exhibitionArtifact(t, store, "artifact-a")
	artifactB := exhibitionArtifact(t, store, "artifact-b")
	svc := exhibitionService(store)

	const contenders = 8
	var wg sync.WaitGroup
	type outcome struct {
		err error
		aid string
	}
	results := make([]outcome, contenders)
	wg.Add(contenders)
	for i := 0; i < contenders; i++ {
		i := i
		go func() {
			defer wg.Done()
			ctx := exhibitionContext(coordinatorA)
			artifactID := artifactA.ID
			if i%2 == 1 {
				ctx = exhibitionContext(coordinatorB)
				artifactID = artifactB.ID
			}
			reserved, err := svc.Reserve(ctx, ReservationInput{
				ArtifactID: artifactID, DisplayCaseID: "case-east-01", Duration: time.Hour,
			})
			results[i] = outcome{err: err, aid: reserved.ArtifactID}
		}()
	}
	wg.Wait()

	successes := 0
	conflicts := 0
	var winnerArtifact string
	for _, r := range results {
		switch {
		case r.err == nil:
			successes++
			winnerArtifact = r.aid
		case errors.Is(r.err, domain.ErrConflict), errors.Is(r.err, domain.ErrVersion):
			conflicts++
		default:
			t.Errorf("unexpected reservation error: %v", r.err)
		}
	}
	if successes != 1 || conflicts != contenders-1 {
		t.Fatalf("expected one winner and %d conflicts, got successes=%d conflicts=%d", contenders-1, successes, conflicts)
	}

	loaded, err := store.GetDisplayCase(context.Background(), coordinatorA.TenantID, "case-east-01")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.CaseReserved || loaded.ArtifactID == "" || loaded.ArtifactID != winnerArtifact {
		t.Fatalf("case not owned by the single winner: %#v", loaded)
	}
}
