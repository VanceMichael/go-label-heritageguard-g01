package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBusPublishClonesPayloadAndUnsubscribes(t *testing.T) {
	bus := New()
	defer bus.Close()
	subscription, err := bus.Subscribe("artifact.updated", 2)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("before")
	if err := bus.Publish(context.Background(), Event{Topic: "artifact.updated", Key: "artifact-1", Body: payload}); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'x'
	select {
	case event := <-subscription.Events:
		if string(event.Body) != "before" {
			t.Fatalf("event payload was not cloned: %q", event.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
	subscription.Close()
	if err := bus.Publish(context.Background(), Event{Topic: "artifact.updated", Body: []byte("ignored")}); err != nil {
		t.Fatal(err)
	}
}

func TestBusPublishHonorsCancellationAndClose(t *testing.T) {
	bus := New()
	subscription, err := bus.Subscribe("slow", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), Event{Topic: "slow", Body: []byte("one")}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := bus.Publish(ctx, Event{Topic: "slow", Body: []byte("two")}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected publish deadline, got %v", err)
	}
	bus.Close()
	if _, err := bus.Subscribe("after-close", 1); err == nil {
		t.Fatal("subscribe after close should fail")
	}
	subscription.Close()
}

func TestBusValidation(t *testing.T) {
	bus := New()
	defer bus.Close()
	for _, capacity := range []int{0, -1, 10001} {
		if _, err := bus.Subscribe("topic", capacity); err == nil {
			t.Errorf("capacity %d should fail", capacity)
		}
	}
	if _, err := bus.Subscribe("", 1); err == nil {
		t.Fatal("empty topic should fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bus.Publish(ctx, Event{Topic: "topic"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled publish, got %v", err)
	}
}
