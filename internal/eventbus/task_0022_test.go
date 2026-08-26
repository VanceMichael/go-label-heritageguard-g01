package eventbus

import (
	"context"
	"testing"
	"time"
)

func TestHeritageGuardTask0022(t *testing.T) {
	bus := New()
	sub, err := bus.Subscribe("environment.backpressure", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if err := bus.Publish(context.Background(), Event{Topic: "environment.backpressure", Key: "first"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- bus.Publish(context.Background(), Event{Topic: "environment.backpressure", Key: "second"}) }()
	time.Sleep(20 * time.Millisecond)
	sub.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("closing a subscriber should release a blocked publisher: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("blocked publisher outlived subscriber cleanup")
	}
}
