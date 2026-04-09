package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncHandler_PanicRecovery(t *testing.T) {
	bus := NewEventBus(nil)

	// Subscribe an async handler that panics
	bus.SubscribeAsync("test.panic", func(_ context.Context, _ Event) error {
		panic("handler exploded")
	})

	// Subscribe a second async handler that should still run
	done := make(chan struct{})
	bus.SubscribeAsync("test.panic", func(_ context.Context, _ Event) error {
		close(done)
		return nil
	})

	// Publish — should not crash the process
	err := bus.Publish(context.Background(), Event{Type: "test.panic"})
	if err != nil {
		t.Fatalf("expected no error from Publish, got %v", err)
	}

	// Wait for the non-panicking handler to complete
	select {
	case <-done:
		// success — both handlers ran, panic was recovered
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for second async handler")
	}

	// Allow the panicking goroutine's deferred recovery to update the error count
	time.Sleep(50 * time.Millisecond)

	// Error count should reflect the panic
	stats := bus.Stats()
	if stats.ErrorCount < 1 {
		t.Errorf("expected error count >= 1, got %d", stats.ErrorCount)
	}
}

func TestEventBus_Drain_WaitsForAsync(t *testing.T) {
	bus := NewEventBus(nil)

	var completed atomic.Int32
	for i := 0; i < 3; i++ {
		bus.SubscribeAsync("test.drain", func(_ context.Context, _ Event) error {
			time.Sleep(100 * time.Millisecond)
			completed.Add(1)
			return nil
		})
	}

	bus.Publish(context.Background(), Event{Type: "test.drain"})
	bus.Drain()

	if completed.Load() != 3 {
		t.Errorf("expected 3 handlers completed after Drain, got %d", completed.Load())
	}
}

func TestAsyncHandler_NoPanic_StillWorks(t *testing.T) {
	bus := NewEventBus(nil)

	done := make(chan string, 1)
	bus.SubscribeAsync("test.ok", func(_ context.Context, e Event) error {
		done <- e.Type
		return nil
	})

	bus.Publish(context.Background(), Event{Type: "test.ok"})

	select {
	case typ := <-done:
		if typ != "test.ok" {
			t.Errorf("expected test.ok, got %s", typ)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async handler")
	}
}
