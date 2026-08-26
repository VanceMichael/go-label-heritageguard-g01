package outbox

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type outboxStore struct {
	event    domain.OutboxEvent
	claimed  bool
	finished []error
}

func (s *outboxStore) ClaimOutbox(_ context.Context, _ string, _ time.Time, _ time.Duration) (domain.OutboxEvent, error) {
	if s.claimed {
		return domain.OutboxEvent{}, domain.ErrNotFound
	}
	s.claimed = true
	return s.event, nil
}

func (s *outboxStore) FinishOutbox(_ context.Context, _ string, _ string, _ time.Time, err error) error {
	s.finished = append(s.finished, err)
	return nil
}

type outboxSender struct {
	err    error
	events []domain.OutboxEvent
}

func (s *outboxSender) Send(ctx context.Context, event domain.OutboxEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.events = append(s.events, event)
	return s.err
}

type outboxLogWriter struct{}

func (outboxLogWriter) Write(p []byte) (int, error) { return len(p), nil }

func outboxLogger() *slog.Logger { return slog.New(slog.NewTextHandler(outboxLogWriter{}, nil)) }

func TestDispatcherDeliversAndFinishesSuccess(t *testing.T) {
	store := &outboxStore{event: domain.OutboxEvent{ID: "event-1", Topic: "incident.opened"}}
	sender := &outboxSender{}
	dispatcher := &Dispatcher{Store: store, Sender: sender, Owner: "dispatcher-1", PollInterval: time.Millisecond, LeaseDuration: time.Second, Now: time.Now, Logger: outboxLogger()}
	dispatcher.dispatchOne(context.Background())
	if len(sender.events) != 1 || len(store.finished) != 1 || store.finished[0] != nil {
		t.Fatalf("successful dispatch lifecycle wrong: events=%#v finished=%#v", sender.events, store.finished)
	}
}

func TestDispatcherRecordsDeliveryFailure(t *testing.T) {
	store := &outboxStore{event: domain.OutboxEvent{ID: "event-2"}}
	failure := errors.New("remote rejected")
	sender := &outboxSender{err: failure}
	dispatcher := &Dispatcher{Store: store, Sender: sender, Owner: "dispatcher-2", PollInterval: time.Millisecond, LeaseDuration: time.Second, Now: time.Now, Logger: outboxLogger()}
	dispatcher.dispatchOne(context.Background())
	if len(store.finished) != 1 || !errors.Is(store.finished[0], failure) {
		t.Fatalf("delivery failure was not finished: %#v", store.finished)
	}
}

func TestMemorySenderClonesPayloadAndHonorsFailure(t *testing.T) {
	sender := &MemorySender{}
	payload := []byte("payload")
	if err := sender.Send(context.Background(), domain.OutboxEvent{ID: "event-3", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'x'
	events := sender.Snapshot()
	if len(events) != 1 || string(events[0].Payload) != "payload" {
		t.Fatalf("sender did not clone payload: %#v", events)
	}
	sender.Fail = errors.New("offline")
	if err := sender.Send(context.Background(), domain.OutboxEvent{ID: "event-4"}); !errors.Is(err, sender.Fail) {
		t.Fatalf("configured sender failure lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sender.Send(ctx, domain.OutboxEvent{ID: "event-5"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled send should fail first: %v", err)
	}
}

func TestDispatcherConfigurationValidation(t *testing.T) {
	dispatcher := &Dispatcher{}
	if err := dispatcher.Run(context.Background()); err == nil {
		t.Fatal("empty dispatcher should fail validation")
	}
	store := &outboxStore{}
	sender := &outboxSender{}
	dispatcher = &Dispatcher{Store: store, Sender: sender, Owner: "owner", Now: time.Now, PollInterval: 0, LeaseDuration: time.Second, Logger: outboxLogger()}
	if err := dispatcher.Run(context.Background()); err == nil {
		t.Fatal("zero poll interval should fail validation")
	}
}
