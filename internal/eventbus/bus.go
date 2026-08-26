package eventbus

import (
	"context"
	"fmt"
	"sync"
)

type Event struct {
	Topic string
	Key   string
	Body  []byte
}

func (e Event) Clone() Event {
	copyEvent := e
	copyEvent.Body = append([]byte(nil), e.Body...)
	return copyEvent
}

type Subscription struct {
	ID     uint64
	Topic  string
	Events <-chan Event
	cancel func()
	once   sync.Once
}

func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(s.cancel)
}

type subscriber struct {
	id     uint64
	topic  string
	events chan Event
	done   chan struct{}
	once   sync.Once
}

func (s *subscriber) close() {
	s.once.Do(func() { close(s.done) })
}

type Bus struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[string]map[uint64]*subscriber
	closed      bool
}

func New() *Bus {
	return &Bus{subscribers: make(map[string]map[uint64]*subscriber)}
}

func (b *Bus) Subscribe(topic string, capacity int) (*Subscription, error) {
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if capacity < 1 || capacity > 10_000 {
		return nil, fmt.Errorf("subscription capacity must be between 1 and 10000")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, fmt.Errorf("event bus is closed")
	}
	b.nextID++
	sub := &subscriber{id: b.nextID, topic: topic, events: make(chan Event, capacity), done: make(chan struct{})}
	if b.subscribers[topic] == nil {
		b.subscribers[topic] = make(map[uint64]*subscriber)
	}
	b.subscribers[topic][sub.id] = sub
	return &Subscription{
		ID:     sub.id,
		Topic:  topic,
		Events: sub.events,
		cancel: func() { b.unsubscribe(sub) },
	}, nil
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("event bus is closed")
	}
	targets := make([]*subscriber, 0, len(b.subscribers[event.Topic]))
	for _, sub := range b.subscribers[event.Topic] {
		targets = append(targets, sub)
	}
	b.mu.RUnlock()
	for _, sub := range targets {
		publishCtx := context.WithoutCancel(ctx)
		if deadline, ok := ctx.Deadline(); ok {
			var cancel context.CancelFunc
			publishCtx, cancel = context.WithDeadline(publishCtx, deadline)
			defer cancel()
		}
		select {
		case <-publishCtx.Done():
			return publishCtx.Err()
		case <-sub.done:
			continue
		case sub.events <- event.Clone():
		}
	}
	return nil
}

func (b *Bus) unsubscribe(sub *subscriber) {
	b.mu.Lock()
	if topic := b.subscribers[sub.topic]; topic != nil {
		delete(topic, sub.id)
		if len(topic) == 0 {
			delete(b.subscribers, sub.topic)
		}
	}
	b.mu.Unlock()
	sub.close()
}

func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	var subscribers []*subscriber
	for _, topic := range b.subscribers {
		for _, sub := range topic {
			subscribers = append(subscribers, sub)
		}
	}
	b.subscribers = make(map[string]map[uint64]*subscriber)
	b.mu.Unlock()
	for _, sub := range subscribers {
		sub.close()
	}
}
