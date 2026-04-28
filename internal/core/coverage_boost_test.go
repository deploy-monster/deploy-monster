package core

import (
	"context"
	"testing"
)

func TestIsDraining(t *testing.T) {
	c := &Core{}
	if c.IsDraining() {
		t.Error("expected not draining initially")
	}
	c.SetDraining()
	if !c.IsDraining() {
		t.Error("expected draining after SetDraining")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus(nil)
	called := false
	handler := func(_ context.Context, _ Event) error {
		called = true
		return nil
	}

	subID := eb.Subscribe("test.event", handler)
	eb.Publish(context.Background(), Event{Type: "test.event"})
	if !called {
		t.Error("handler should be called before unsubscribe")
	}

	called = false
	eb.Unsubscribe(subID)
	eb.Publish(context.Background(), Event{Type: "test.event"})
	if called {
		t.Error("handler should NOT be called after unsubscribe")
	}
}

func TestWithCorrelationID(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "corr-123")
	if CorrelationIDFromContext(ctx) != "corr-123" {
		t.Errorf("expected correlation ID corr-123, got %s", CorrelationIDFromContext(ctx))
	}
}

func TestCorrelationIDFromContext_Empty(t *testing.T) {
	if CorrelationIDFromContext(context.Background()) != "" {
		t.Error("expected empty correlation ID from background context")
	}
}

func TestContainerOpts_ApplyResourceDefaults(t *testing.T) {
	co := &ContainerOpts{}
	co.ApplyResourceDefaults(512, 512)
	if co.CPUQuota != 512 {
		t.Errorf("CPUQuota = %d, want 512", co.CPUQuota)
	}
	if co.MemoryMB != 512 {
		t.Errorf("MemoryMB = %d, want 512", co.MemoryMB)
	}

	// When values are already set, defaults should not override
	co2 := &ContainerOpts{CPUQuota: 1000, MemoryMB: 2048}
	co2.ApplyResourceDefaults(512, 512)
	if co2.CPUQuota != 1000 {
		t.Errorf("CPUQuota = %d, want 1000", co2.CPUQuota)
	}
	if co2.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", co2.MemoryMB)
	}

	// When defaults are 0, existing zero values stay zero
	co3 := &ContainerOpts{}
	co3.ApplyResourceDefaults(0, 0)
	if co3.CPUQuota != 0 {
		t.Errorf("CPUQuota = %d, want 0", co3.CPUQuota)
	}
	if co3.MemoryMB != 0 {
		t.Errorf("MemoryMB = %d, want 0", co3.MemoryMB)
	}
}
