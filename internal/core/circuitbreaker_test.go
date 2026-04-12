package core

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState_AllowsCalls(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Error("expected function to be called")
	}
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     1 * time.Minute,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)
	testErr := errors.New("service down")

	// 3 failures should trip the circuit
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error { return testErr })
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected open after %d failures, got %s", 3, cb.State())
	}

	// Next call should be rejected
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_ResetTimeoutTransitionsToHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	now := time.Now()
	cb.now = func() time.Time { return now }

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	// Advance time past reset timeout
	cb.now = func() time.Time { return now.Add(200 * time.Millisecond) }

	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected half-open after timeout, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	now := time.Now()
	cb.now = func() time.Time { return now }

	// Trip it
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}

	// Advance past timeout
	cb.now = func() time.Time { return now.Add(200 * time.Millisecond) }

	// Successful probe should close the circuit
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected probe to succeed, got %v", err)
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after successful probe, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	now := time.Now()
	cb.now = func() time.Time { return now }

	// Trip it
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}

	// Advance past timeout into half-open
	now2 := now.Add(200 * time.Millisecond)
	cb.now = func() time.Time { return now2 }

	// Failed probe should reopen
	_ = cb.Execute(func() error { return errors.New("still failing") })

	if cb.State() != CircuitOpen {
		t.Errorf("expected open after failed probe, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_LimitsProbes(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	now := time.Now()
	cb.now = func() time.Time { return now }

	// Trip it
	_ = cb.Execute(func() error { return errors.New("fail") })

	// Advance past timeout
	cb.now = func() time.Time { return now.Add(200 * time.Millisecond) }

	// First half-open call — allowed as probe
	// We need to simulate it being in-flight by locking directly
	cb.mu.Lock()
	cb.halfOpenCalls = 1
	cb.state = CircuitHalfOpen
	cb.mu.Unlock()

	// Second call should be rejected
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen for excess half-open calls, got %v", err)
	}
}

func TestCircuitBreaker_NoRetryError_DoesNotCount(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     1 * time.Minute,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Non-retryable errors (e.g., 404) should not count toward failures
	for i := 0; i < 10; i++ {
		_ = cb.Execute(func() error {
			return ErrNoRetry(errors.New("not found"))
		})
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed (non-retryable errors), got %s", cb.State())
	}
}

func TestCircuitBreaker_SuccessResetFailureCount(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     1 * time.Minute,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// 2 failures then a success
	_ = cb.Execute(func() error { return errors.New("fail") })
	_ = cb.Execute(func() error { return errors.New("fail") })
	_ = cb.Execute(func() error { return nil })

	// Failure count should be reset
	stats := cb.Stats()
	if stats.Failures != 0 {
		t.Errorf("expected 0 failures after success, got %d", stats.Failures)
	}
	if stats.State != CircuitClosed {
		t.Errorf("expected closed, got %s", stats.State)
	}
}

func TestCircuitBreaker_ManualReset(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     1 * time.Hour,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	_ = cb.Execute(func() error { return errors.New("fail") })

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after manual reset, got %s", cb.State())
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker("github-api", DefaultCircuitBreakerConfig())

	stats := cb.Stats()
	if stats.Name != "github-api" {
		t.Errorf("expected name github-api, got %s", stats.Name)
	}
	if stats.State != CircuitClosed {
		t.Errorf("expected closed, got %s", stats.State)
	}
	if stats.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", stats.Failures)
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	if cfg.FailureThreshold != 5 {
		t.Errorf("expected threshold 5, got %d", cfg.FailureThreshold)
	}
	if cfg.ResetTimeout != 30*time.Second {
		t.Errorf("expected 30s reset, got %v", cfg.ResetTimeout)
	}
	if cfg.HalfOpenMaxCalls != 1 {
		t.Errorf("expected 1 half-open call, got %d", cfg.HalfOpenMaxCalls)
	}
}

func TestCircuitBreaker_InvalidConfig_Defaults(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{})
	if cb.failureThreshold != 5 {
		t.Errorf("expected default threshold 5, got %d", cb.failureThreshold)
	}
	if cb.resetTimeout != 30*time.Second {
		t.Errorf("expected default 30s reset, got %v", cb.resetTimeout)
	}
	if cb.halfOpenMax != 1 {
		t.Errorf("expected default 1 half-open call, got %d", cb.halfOpenMax)
	}
}

func TestCircuitBreaker_NilSafe(t *testing.T) {
	var cb *CircuitBreaker

	// Execute should pass through
	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected function to be called")
	}

	// State and Stats should not panic
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed for nil CB, got %s", cb.State())
	}
	stats := cb.Stats()
	if stats.State != CircuitClosed {
		t.Errorf("expected closed state in stats, got %s", stats.State)
	}
}
