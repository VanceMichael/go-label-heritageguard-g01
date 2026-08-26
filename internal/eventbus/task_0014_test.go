package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHeritageGuardTask0014(t *testing.T) {
	bus := New()
	sub, err := bus.Subscribe("environment.incident", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if err := bus.Publish(context.Background(), Event{Topic: "environment.incident", Key: "first"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bus.Publish(ctx, Event{Topic: "environment.incident", Key: "second"}) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("backpressured publish returned the wrong error: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancelled backpressured publish did not return")
	}
	_ = sub
}
