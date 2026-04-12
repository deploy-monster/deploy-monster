package graceful

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_New(t *testing.T) {
	config := DefaultCircuitConfig()
	cb := NewCircuitBreaker(config)

	if cb == nil {
		t.Fatal("NewCircuitBreaker returned nil")
	}
	if cb.State() != CircuitClosed {
		t.Errorf("State() = %v, want %v", cb.State(), CircuitClosed)
	}
}

func TestCircuitBreaker_AllowRequest_Closed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	})

	// In closed state, all requests should be allowed
	for i := 0; i < 10; i++ {
		if !cb.AllowRequest() {
			t.Errorf("AllowRequest() = false in closed state, iteration %d", i)
		}
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	})

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("State() = %v, want %v after %d failures", cb.State(), CircuitOpen, 3)
	}

	// Requests should be denied
	if cb.AllowRequest() {
		t.Error("AllowRequest() should return false in open state")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("State() = %v, want %v", cb.State(), CircuitOpen)
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open on next request
	if !cb.AllowRequest() {
		t.Error("AllowRequest() should return true after timeout (half-open)")
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("State() = %v, want %v", cb.State(), CircuitHalfOpen)
	}
}

func TestCircuitBreaker_CloseAfterSuccessesInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)
	cb.AllowRequest()

	if cb.State() != CircuitHalfOpen {
		t.Fatalf("State() = %v, want %v", cb.State(), CircuitHalfOpen)
	}

	// Record successes to close
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Errorf("State() = %v, want %v after 1 success", cb.State(), CircuitHalfOpen)
	}

	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("State() = %v, want %v after 2 successes", cb.State(), CircuitClosed)
	}
}

func TestCircuitBreaker_OpenOnFailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)
	cb.AllowRequest()

	if cb.State() != CircuitHalfOpen {
		t.Fatalf("State() = %v, want %v", cb.State(), CircuitHalfOpen)
	}

	// Any failure should open again
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("State() = %v, want %v after failure in half-open", cb.State(), CircuitOpen)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	})

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("State() = %v, want %v", cb.State(), CircuitOpen)
	}

	// Reset
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("State() = %v, want %v after reset", cb.State(), CircuitClosed)
	}

	if !cb.AllowRequest() {
		t.Error("AllowRequest() should return true after reset")
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	})

	// Record some failures (but not enough to open)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.FailureCount != 3 {
		t.Errorf("FailureCount = %d, want 3", stats.FailureCount)
	}

	// Success should reset failure count
	cb.RecordSuccess()

	stats = cb.Stats()
	if stats.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0 after success", stats.FailureCount)
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	})

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.State != CircuitClosed {
		t.Errorf("State = %v, want %v", stats.State, CircuitClosed)
	}
	if stats.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", stats.FailureCount)
	}
	if stats.LastFailTime.IsZero() {
		t.Error("LastFailTime should be set")
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 100,
		SuccessThreshold: 50,
		Timeout:          1 * time.Second,
	})

	var wg sync.WaitGroup

	// Concurrent successes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.RecordSuccess()
			}
		}()
	}

	// Concurrent failures
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cb.RecordFailure()
			}
		}()
	}

	// Concurrent AllowRequest checks
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.AllowRequest()
			}
		}()
	}

	wg.Wait()
	// Should not panic or race
}

func TestCircuitBreakerState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CircuitBreakerManager Tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestCircuitBreakerManager_New(t *testing.T) {
	config := DefaultCircuitConfig()
	m := NewCircuitBreakerManager(config)

	if m == nil {
		t.Fatal("NewCircuitBreakerManager returned nil")
	}
}

func TestCircuitBreakerManager_AllowRequest(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	})

	// New backend should allow requests
	if !m.AllowRequest("backend-1") {
		t.Error("AllowRequest() should return true for new backend")
	}

	// Same backend should still allow
	if !m.AllowRequest("backend-1") {
		t.Error("AllowRequest() should return true for existing backend in closed state")
	}
}

func TestCircuitBreakerManager_RecordFailure(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// Record failures for backend
	m.RecordFailure("backend-1")
	m.RecordFailure("backend-1")

	if !m.IsOpen("backend-1") {
		t.Error("IsOpen() should return true after failures reach threshold")
	}

	// Different backend should not be affected
	if m.IsOpen("backend-2") {
		t.Error("IsOpen() should return false for unaffected backend")
	}
}

func TestCircuitBreakerManager_RecordSuccess(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// Open the circuit
	m.RecordFailure("backend-1")
	m.RecordFailure("backend-1")

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Allow request triggers half-open
	m.AllowRequest("backend-1")

	// Record successes to close
	m.RecordSuccess("backend-1")
	m.RecordSuccess("backend-1")

	if m.IsOpen("backend-1") {
		t.Error("IsOpen() should return false after successful recovery")
	}
}

func TestCircuitBreakerManager_Reset(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	})

	// Open the circuit
	m.RecordFailure("backend-1")
	m.RecordFailure("backend-1")

	if !m.IsOpen("backend-1") {
		t.Fatal("Circuit should be open")
	}

	// Reset
	m.Reset("backend-1")

	if m.IsOpen("backend-1") {
		t.Error("IsOpen() should return false after reset")
	}
}

func TestCircuitBreakerManager_Stats(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	})

	// Non-existent backend
	_, ok := m.Stats("nonexistent")
	if ok {
		t.Error("Stats() should return false for non-existent backend")
	}

	// Existing backend
	m.RecordFailure("backend-1")
	stats, ok := m.Stats("backend-1")
	if !ok {
		t.Fatal("Stats() should return true for existing backend")
	}
	if stats.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", stats.FailureCount)
	}
}

func TestCircuitBreakerManager_AllStats(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	})

	m.RecordFailure("backend-1")
	m.RecordFailure("backend-2")
	// RecordSuccess on non-existent backend doesn't create a breaker
	// Only failures create breakers automatically
	m.RecordFailure("backend-3")
	m.RecordSuccess("backend-3") // This will reset failure count

	allStats := m.AllStats()
	if len(allStats) != 3 {
		t.Errorf("AllStats() returned %d entries, want 3", len(allStats))
	}

	if allStats["backend-1"].FailureCount != 1 {
		t.Errorf("backend-1 FailureCount = %d, want 1", allStats["backend-1"].FailureCount)
	}
	if allStats["backend-2"].FailureCount != 1 {
		t.Errorf("backend-2 FailureCount = %d, want 1", allStats["backend-2"].FailureCount)
	}
	// backend-3 had failure then success, so failure count is 0
	if allStats["backend-3"].FailureCount != 0 {
		t.Errorf("backend-3 FailureCount = %d, want 0", allStats["backend-3"].FailureCount)
	}
}

func TestCircuitBreakerManager_IsAvailable(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// Initially available
	if !m.IsAvailable("backend-1") {
		t.Error("IsAvailable() should return true for new backend")
	}

	// Open the circuit
	m.RecordFailure("backend-1")
	m.RecordFailure("backend-1")

	if m.IsAvailable("backend-1") {
		t.Error("IsAvailable() should return false when circuit is open")
	}

	// Wait for timeout - should be available in half-open
	time.Sleep(60 * time.Millisecond)
	if !m.IsAvailable("backend-1") {
		t.Error("IsAvailable() should return true in half-open state")
	}
}

func TestCircuitBreakerManager_Concurrent(t *testing.T) {
	m := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 100,
		SuccessThreshold: 50,
		Timeout:          1 * time.Second,
	})

	var wg sync.WaitGroup
	backends := []string{"b1", "b2", "b3"}

	// Concurrent operations on multiple backends
	for _, backend := range backends {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(b string) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					m.AllowRequest(b)
					m.RecordSuccess(b)
					m.RecordFailure(b)
					m.State(b)
					m.IsOpen(b)
					m.IsAvailable(b)
				}
			}(backend)
		}
	}

	// Concurrent stats operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.AllStats()
				m.Stats("b1")
			}
		}()
	}

	wg.Wait()
	// Should not panic or race
}
