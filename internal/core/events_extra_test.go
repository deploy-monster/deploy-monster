package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_OnError(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var captured struct {
		mu    sync.Mutex
		event Event
		sub   *Subscription
		err   error
	}

	eb.OnError(func(event Event, sub *Subscription, err error) {
		captured.mu.Lock()
		defer captured.mu.Unlock()
		captured.event = event
		captured.sub = sub
		captured.err = err
	})

	handlerErr := errors.New("handler exploded")
	eb.SubscribeNamed("test.error", "failing-handler", false, func(_ context.Context, _ Event) error {
		return handlerErr
	})

	err := eb.Publish(context.Background(), Event{Type: "test.error", Source: "test"})
	if err == nil {
		t.Fatal("expected error from sync handler, got nil")
	}

	captured.mu.Lock()
	defer captured.mu.Unlock()

	if captured.err == nil {
		t.Fatal("OnError callback was not called")
	}
	if !errors.Is(captured.err, handlerErr) {
		t.Errorf("OnError got error %v, want %v", captured.err, handlerErr)
	}
	if captured.event.Type != "test.error" {
		t.Errorf("OnError got event type %q, want %q", captured.event.Type, "test.error")
	}
	if captured.sub.Name != "failing-handler" {
		t.Errorf("OnError got handler name %q, want %q", captured.sub.Name, "failing-handler")
	}

	// Verify error count is incremented
	stats := eb.Stats()
	if stats.ErrorCount != 1 {
		t.Errorf("expected ErrorCount 1, got %d", stats.ErrorCount)
	}
}

func TestEventBus_OnError_AsyncHandler(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var errorCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	eb.OnError(func(_ Event, _ *Subscription, _ error) {
		errorCalled.Store(true)
		wg.Done()
	})

	eb.SubscribeAsync("test.async.error", func(_ context.Context, _ Event) error {
		return errors.New("async failure")
	})

	// PublishAsync should not block and should not return the async error
	err := eb.Publish(context.Background(), Event{Type: "test.async.error", Source: "test"})
	if err != nil {
		t.Fatalf("async handler error should not propagate, got: %v", err)
	}

	// Wait for async handler to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async error callback")
	}

	if !errorCalled.Load() {
		t.Error("OnError should be called for async handler failures")
	}
}

func TestEventBus_PublishAsync(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var received atomic.Bool

	eb.Subscribe("test.async.publish", func(_ context.Context, _ Event) error {
		received.Store(true)
		return nil
	})

	// PublishAsync should not block
	start := time.Now()
	eb.PublishAsync(context.Background(), Event{Type: "test.async.publish", Source: "test"})
	elapsed := time.Since(start)

	// The call should return almost immediately (not block on handler)
	if elapsed > 500*time.Millisecond {
		t.Errorf("PublishAsync took %v, expected near-instant return", elapsed)
	}

	// Wait for the async goroutine to finish
	time.Sleep(200 * time.Millisecond)

	if !received.Load() {
		t.Error("handler should eventually be called by PublishAsync")
	}

	stats := eb.Stats()
	if stats.PublishCount != 1 {
		t.Errorf("expected PublishCount 1, got %d", stats.PublishCount)
	}
}

func TestEventBus_EmitWithTenant(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		userID   string
		data     any
	}{
		{
			name:     "with tenant and user",
			tenantID: "tenant-abc",
			userID:   "user-123",
			data:     map[string]string{"key": "value"},
		},
		{
			name:     "system event with empty tenant",
			tenantID: "",
			userID:   "",
			data:     nil,
		},
		{
			name:     "with tenant only",
			tenantID: "tenant-xyz",
			userID:   "",
			data:     "simple-payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eb := NewEventBus(slog.Default())

			var received Event
			eb.Subscribe("tenant.test", func(_ context.Context, e Event) error {
				received = e
				return nil
			})

			err := eb.EmitWithTenant(
				context.Background(),
				"tenant.test", "source-mod",
				tt.tenantID, tt.userID, tt.data,
			)
			if err != nil {
				t.Fatalf("EmitWithTenant returned error: %v", err)
			}

			if received.Type != "tenant.test" {
				t.Errorf("expected type %q, got %q", "tenant.test", received.Type)
			}
			if received.Source != "source-mod" {
				t.Errorf("expected source %q, got %q", "source-mod", received.Source)
			}
			if received.TenantID != tt.tenantID {
				t.Errorf("expected tenantID %q, got %q", tt.tenantID, received.TenantID)
			}
			if received.UserID != tt.userID {
				t.Errorf("expected userID %q, got %q", tt.userID, received.UserID)
			}
		})
	}
}

func TestEvent_DebugString(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		wantSub  []string // substrings expected in the output
	}{
		{
			name: "full event with all fields",
			event: Event{
				ID:       "abcdef1234567890",
				Type:     "app.deployed",
				Source:   "deploy-module",
				TenantID: "tenant-1",
				UserID:   "user-42",
			},
			wantSub: []string{
				"abcdef12",       // first 8 chars of ID
				"app.deployed",   // event type
				"deploy-module",  // source
				"tenant=tenant-1",
				"user=user-42",
			},
		},
		{
			name: "system event with empty tenant and user",
			event: Event{
				ID:       "1234567890abcdef",
				Type:     "system.started",
				Source:   "core",
				TenantID: "",
				UserID:   "",
			},
			wantSub: []string{
				"12345678",
				"system.started",
				"core",
				"tenant=",
				"user=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.DebugString()
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("DebugString() = %q, missing substring %q", got, sub)
				}
			}
		})
	}

	// Verify format matches expected pattern
	e := Event{
		ID:       "abcdef1234567890",
		Type:     "app.created",
		Source:   "api",
		TenantID: "t1",
		UserID:   "u1",
	}
	expected := fmt.Sprintf("[%s] %s from %s (tenant=%s user=%s)",
		e.ID[:8], e.Type, e.Source, e.TenantID, e.UserID)
	if got := e.DebugString(); got != expected {
		t.Errorf("DebugString() = %q, want %q", got, expected)
	}
}
