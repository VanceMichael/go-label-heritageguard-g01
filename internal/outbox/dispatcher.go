package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type Store interface {
	ClaimOutbox(context.Context, string, time.Time, time.Duration) (domain.OutboxEvent, error)
	FinishOutbox(context.Context, string, string, time.Time, error) error
}

type Sender interface {
	Send(context.Context, domain.OutboxEvent) error
}

type Dispatcher struct {
	Store         Store
	Sender        Sender
	Owner         string
	PollInterval  time.Duration
	LeaseDuration time.Duration
	Now           func() time.Time
	Logger        *slog.Logger
}

func (d *Dispatcher) Run(ctx context.Context) error {
	if d.Store == nil || d.Sender == nil || d.Owner == "" || d.Now == nil {
		return fmt.Errorf("outbox dispatcher is not configured")
	}
	if d.PollInterval <= 0 || d.LeaseDuration <= 0 {
		return fmt.Errorf("outbox intervals must be positive")
	}
	ticker := time.NewTicker(d.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			d.dispatchOne(ctx)
		}
	}
}

func (d *Dispatcher) dispatchOne(ctx context.Context) {
	event, err := d.Store.ClaimOutbox(ctx, d.Owner, d.Now(), d.LeaseDuration)
	if err != nil {
		if err != domain.ErrNotFound {
			d.Logger.Error("claim outbox event", "error", err)
		}
		return
	}
	deliveryErr := d.Sender.Send(ctx, event)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := d.Store.FinishOutbox(finishCtx, event.ID, d.Owner, d.Now(), deliveryErr); err != nil {
		d.Logger.Error("finish outbox event", "event_id", event.ID, "error", err)
	}
}

type HTTPSender struct {
	Client   *http.Client
	Endpoint string
}

func (s HTTPSender) Send(ctx context.Context, event domain.OutboxEvent) error {
	if s.Client == nil || s.Endpoint == "" {
		return fmt.Errorf("HTTP sender is not configured")
	}
	payload, err := json.Marshal(map[string]any{
		"id": event.ID, "topic": event.Topic, "aggregate_id": event.AggregateID, "payload": json.RawMessage(event.Payload),
	})
	if err != nil {
		return fmt.Errorf("encode outbox delivery: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create outbox request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", event.IdempotencyKey)
	resp, err := s.Client.Do(req)
	if err != nil {
		return domain.DependencyError{Operation: "deliver outbox event", Err: err}
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if readErr != nil {
		return domain.DependencyError{Operation: "read outbox response", Err: readErr}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.DependencyError{Operation: "deliver outbox event", Err: fmt.Errorf("status %d: %s", resp.StatusCode, string(body))}
	}
	return nil
}

type MemorySender struct {
	mu     sync.Mutex
	Events []domain.OutboxEvent
	Fail   error
}

func (s *MemorySender) Send(ctx context.Context, event domain.OutboxEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail != nil {
		return s.Fail
	}
	copyEvent := event
	copyEvent.Payload = append([]byte(nil), event.Payload...)
	s.Events = append(s.Events, copyEvent)
	return nil
}

func (s *MemorySender) Snapshot() []domain.OutboxEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]domain.OutboxEvent, len(s.Events))
	copy(result, s.Events)
	return result
}
