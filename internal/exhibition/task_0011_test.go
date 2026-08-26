package exhibition

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0011(t *testing.T) {
	store := exhibitionStore(t)
	_ = exhibitionUser(t, store, "coordinator-task-0011", domain.RoleCoordinator)
	artifact := exhibitionArtifact(t, store, "artifact-task-0011")
	item, err := store.GetDisplayCase(context.Background(), "museum-demo", "case-east-01")
	if err != nil {
		t.Fatal(err)
	}
	item.ArtifactID = artifact.ID
	item.Status = domain.CaseReserved
	reservationTo := store.Now().Add(time.Hour)
	item.ReservationTo = &reservationTo
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			err := store.ReserveDisplayCase(context.Background(), item, item.Version)
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("display case ownership was not claimed by exactly one request: successes=%d", successes)
	}
}
