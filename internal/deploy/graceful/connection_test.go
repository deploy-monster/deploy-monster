package graceful

import (
	"sync"
	"testing"
	"time"
)

func TestConnectionTracker_New(t *testing.T) {
	ct := NewConnectionTracker()
	if ct == nil {
		t.Fatal("NewConnectionTracker returned nil")
	}
	if ct.conns == nil {
		t.Error("conns map should be initialized")
	}
}

func TestConnectionTracker_Increment(t *testing.T) {
	ct := NewConnectionTracker()

	// First increment should create entry and set to 1
	ct.Increment("container-1")
	if got := ct.Active("container-1"); got != 1 {
		t.Errorf("Active(container-1) = %d, want 1", got)
	}

	// Second increment should increase to 2
	ct.Increment("container-1")
	if got := ct.Active("container-1"); got != 2 {
		t.Errorf("Active(container-1) = %d, want 2", got)
	}

	// Different container should have separate counter
	ct.Increment("container-2")
	if got := ct.Active("container-2"); got != 1 {
		t.Errorf("Active(container-2) = %d, want 1", got)
	}
	if got := ct.Active("container-1"); got != 2 {
		t.Errorf("Active(container-1) should still be 2, got %d", got)
	}
}

func TestConnectionTracker_Decrement(t *testing.T) {
	ct := NewConnectionTracker()

	// Increment twice
	ct.Increment("container-1")
	ct.Increment("container-1")

	// Decrement once
	ct.Decrement("container-1")
	if got := ct.Active("container-1"); got != 1 {
		t.Errorf("Active(container-1) = %d, want 1", got)
	}

	// Decrement to zero
	ct.Decrement("container-1")
	if got := ct.Active("container-1"); got != 0 {
		t.Errorf("Active(container-1) = %d, want 0", got)
	}

	// Decrement below zero should not go negative
	ct.Decrement("container-1")
	if got := ct.Active("container-1"); got != 0 {
		t.Errorf("Active(container-1) = %d, want 0 (should not go negative)", got)
	}
}

func TestConnectionTracker_Active_NonExistent(t *testing.T) {
	ct := NewConnectionTracker()

	if got := ct.Active("nonexistent"); got != 0 {
		t.Errorf("Active(nonexistent) = %d, want 0", got)
	}
}

func TestConnectionTracker_Concurrent(t *testing.T) {
	ct := NewConnectionTracker()
	var wg sync.WaitGroup

	// 100 goroutines increment 100 times each = 10000
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ct.Increment("container-concurrent")
			}
		}()
	}

	wg.Wait()

	if got := ct.Active("container-concurrent"); got != 10000 {
		t.Errorf("Active(container-concurrent) = %d, want 10000", got)
	}

	// Now decrement
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ct.Decrement("container-concurrent")
			}
		}()
	}

	wg.Wait()

	if got := ct.Active("container-concurrent"); got != 0 {
		t.Errorf("Active(container-concurrent) = %d, want 0", got)
	}
}

func TestDrainManager_New(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	if dm == nil {
		t.Fatal("NewDrainManager returned nil")
	}
	if dm.draining == nil {
		t.Error("draining map should be initialized")
	}
}

func TestDrainManager_StartDrain(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	done := dm.StartDrain("container-1")
	if done == nil {
		t.Error("StartDrain should return a channel")
	}

	// Starting drain again should return nil (already draining)
	done2 := dm.StartDrain("container-1")
	if done2 != nil {
		t.Error("StartDrain on already draining container should return nil")
	}
}

func TestDrainManager_IsDraining(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	if dm.IsDraining("container-1") {
		t.Error("IsDraining should be false before StartDrain")
	}

	dm.StartDrain("container-1")

	if !dm.IsDraining("container-1") {
		t.Error("IsDraining should be true after StartDrain")
	}

	dm.CompleteDrain("container-1")

	if dm.IsDraining("container-1") {
		t.Error("IsDraining should be false after CompleteDrain")
	}
}

func TestDrainManager_CompleteDrain(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	done := dm.StartDrain("container-1")

	// Complete the drain
	dm.CompleteDrain("container-1")

	// Channel should be closed
	select {
	case <-done:
		// Good - channel is closed
	default:
		t.Error("done channel should be closed after CompleteDrain")
	}
}

func TestDrainManager_CompleteDrain_NonExistent(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	// Should not panic
	dm.CompleteDrain("nonexistent")
}

func TestDrainManager_WaitForDrain_Immediate(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	// No active connections, should return immediately
	err := dm.WaitForDrain("container-1", 1*time.Second)
	if err != nil {
		t.Errorf("WaitForDrain with no connections should succeed, got: %v", err)
	}
}

func TestDrainManager_WaitForDrain_WithConnections(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	// Add some connections
	ct.Increment("container-1")
	ct.Increment("container-1")

	// Start waiting in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- dm.WaitForDrain("container-1", 2*time.Second)
	}()

	// Wait a bit, then decrement connections
	time.Sleep(100 * time.Millisecond)
	ct.Decrement("container-1")
	ct.Decrement("container-1")

	// Should complete without timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("WaitForDrain should succeed, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("WaitForDrain should complete after connections reach zero")
	}
}

func TestDrainManager_WaitForDrain_Timeout(t *testing.T) {
	ct := NewConnectionTracker()
	dm := NewDrainManager(ct)

	// Add connections that won't be decremented
	ct.Increment("container-1")

	err := dm.WaitForDrain("container-1", 100*time.Millisecond)
	if err != ErrDrainTimeout {
		t.Errorf("WaitForDrain should return ErrDrainTimeout, got: %v", err)
	}
}

func TestDrainState(t *testing.T) {
	ct := NewConnectionTracker()
	ct.Increment("container-1")
	ct.Increment("container-1")

	dm := NewDrainManager(ct)
	dm.StartDrain("container-1")

	dm.mu.RLock()
	state, ok := dm.draining["container-1"]
	dm.mu.RUnlock()

	if !ok {
		t.Fatal("drain state not found")
	}

	if state.ContainerID != "container-1" {
		t.Errorf("ContainerID = %q, want %q", state.ContainerID, "container-1")
	}

	if state.ActiveConns != 2 {
		t.Errorf("ActiveConns = %d, want 2", state.ActiveConns)
	}

	if state.Done == nil {
		t.Error("Done channel should not be nil")
	}

	if state.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
}
