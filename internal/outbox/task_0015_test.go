package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type task0015Store struct{ finishErr error }

func (s *task0015Store) ClaimOutbox(context.Context, string, time.Time, time.Duration) (domain.OutboxEvent, error) {
	return domain.OutboxEvent{ID: "event-task-0015"}, nil
}
func (s *task0015Store) FinishOutbox(ctx context.Context, _, _ string, _ time.Time, _ error) error {
	s.finishErr = ctx.Err()
	return nil
}

type task0015Sender struct{ cancel context.CancelFunc }

func (s task0015Sender) Send(context.Context, domain.OutboxEvent) error {
	s.cancel()
	return errors.New("remote timeout")
}

func TestHeritageGuardTask0015(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &task0015Store{}
	dispatcher := &Dispatcher{Store: store, Sender: task0015Sender{cancel: cancel}, Owner: "owner-task-0015", PollInterval: time.Millisecond, LeaseDuration: time.Second, Now: time.Now, Logger: outboxLogger()}
	dispatcher.dispatchOne(ctx)
	if store.finishErr != nil {
		t.Fatalf("outbox lease cleanup must use a completion context after delivery cancellation: %v", store.finishErr)
	}
}
