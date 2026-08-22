package loan

import (
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func TestHeritageGuardTask0018(t *testing.T) {
	store := loanStore(t)
	registrar := loanUser(t, store, "registrar-task-0018", domain.RoleRegistrar)
	supervisor := loanUser(t, store, "supervisor-task-0018", domain.RoleSupervisor)
	artifact := loanArtifact(t, store, "artifact-task-0018")
	service := loanService(store)
	start, end := loanPeriod(store)
	first, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum A", Purpose: "spring exhibit", StartAt: start, EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(loanContext(registrar), CreateInput{ArtifactID: artifact.ID, Borrower: "Museum B", Purpose: "autumn exhibit", StartAt: end.Add(time.Hour), EndAt: end.Add(48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Submit(loanContext(registrar), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Submit(loanContext(registrar), second.ID); err != nil {
		t.Fatal(err)
	}
	startGate := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, id := range []string{first.ID, second.ID} {
		group.Add(1)
		go func(loanID string) {
			defer group.Done()
			<-startGate
			_, approveErr := service.Approve(loanContext(supervisor), loanID)
			results <- approveErr
		}(id)
	}
	close(startGate)
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("artifact approval ownership was not exclusive: successes=%d", successes)
	}
}
