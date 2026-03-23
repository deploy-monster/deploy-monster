package core

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_Subscribe_ExactMatch(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var called bool

	eb.Subscribe("app.created", func(_ context.Context, e Event) error {
		called = true
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "app.created", Source: "test"})

	if !called {
		t.Error("handler was not called for exact match")
	}
}

func TestEventBus_Subscribe_NoMatch(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var called bool

	eb.Subscribe("app.created", func(_ context.Context, _ Event) error {
		called = true
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "app.deleted", Source: "test"})

	if called {
		t.Error("handler should not be called for non-matching event")
	}
}

func TestEventBus_Subscribe_Wildcard(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var count int

	eb.Subscribe("*", func(_ context.Context, _ Event) error {
		count++
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "app.created"})
	eb.Publish(context.Background(), Event{Type: "build.started"})
	eb.Publish(context.Background(), Event{Type: "anything"})

	if count != 3 {
		t.Errorf("wildcard handler called %d times, expected 3", count)
	}
}

func TestEventBus_Subscribe_PrefixMatch(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var matched []string

	eb.Subscribe("app.*", func(_ context.Context, e Event) error {
		matched = append(matched, e.Type)
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "app.created"})
	eb.Publish(context.Background(), Event{Type: "app.deployed"})
	eb.Publish(context.Background(), Event{Type: "build.started"})

	if len(matched) != 2 {
		t.Errorf("prefix handler matched %d events, expected 2", len(matched))
	}
}

func TestEventBus_Publish_SetsTimestamp(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var got time.Time

	eb.Subscribe("test", func(_ context.Context, e Event) error {
		got = e.Timestamp
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "test"})

	if got.IsZero() {
		t.Error("timestamp should be set on publish")
	}
}

func TestEventBus_Publish_SetsID(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var got string

	eb.Subscribe("test", func(_ context.Context, e Event) error {
		got = e.ID
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "test"})

	if got == "" {
		t.Error("event ID should be auto-generated")
	}
}

func TestEventBus_SubscribeAsync(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var called atomic.Bool

	eb.SubscribeAsync("test", func(_ context.Context, _ Event) error {
		called.Store(true)
		return nil
	})

	eb.Publish(context.Background(), Event{Type: "test"})

	// Give async handler time to execute
	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("async handler was not called")
	}
}

func TestEventBus_Stats(t *testing.T) {
	eb := NewEventBus(slog.Default())
	eb.Subscribe("a", func(_ context.Context, _ Event) error { return nil })
	eb.Subscribe("b", func(_ context.Context, _ Event) error { return nil })

	eb.Publish(context.Background(), Event{Type: "a"})
	eb.Publish(context.Background(), Event{Type: "b"})

	stats := eb.Stats()
	if stats.SubscriptionCount != 2 {
		t.Errorf("expected 2 subscriptions, got %d", stats.SubscriptionCount)
	}
	if stats.PublishCount != 2 {
		t.Errorf("expected 2 publishes, got %d", stats.PublishCount)
	}
}

func TestEventBus_Emit(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var receivedData any

	eb.Subscribe("test.event", func(_ context.Context, e Event) error {
		receivedData = e.Data
		return nil
	})

	eb.Emit(context.Background(), "test.event", "test-module", map[string]string{"key": "value"})

	if receivedData == nil {
		t.Error("Emit should deliver data to handler")
	}
}

func TestNewEvent(t *testing.T) {
	e := NewEvent("app.created", "api", AppEventData{AppID: "123"})

	if e.Type != "app.created" {
		t.Errorf("expected type 'app.created', got %q", e.Type)
	}
	if e.Source != "api" {
		t.Errorf("expected source 'api', got %q", e.Source)
	}
	if e.ID == "" {
		t.Error("expected non-empty ID")
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewTenantEvent(t *testing.T) {
	e := NewTenantEvent("app.deployed", "deploy", "tenant-1", "user-1", nil)

	if e.TenantID != "tenant-1" {
		t.Errorf("expected tenant 'tenant-1', got %q", e.TenantID)
	}
	if e.UserID != "user-1" {
		t.Errorf("expected user 'user-1', got %q", e.UserID)
	}
}
