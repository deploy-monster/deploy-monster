package core

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Circuit breaker states.
const (
	CircuitClosed   = "closed"
	CircuitOpen     = "open"
	CircuitHalfOpen = "half-open"
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreakerConfig configures circuit breaker thresholds.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures to trip the circuit.
	FailureThreshold int
	// ResetTimeout is how long to wait in open state before trying half-open.
	ResetTimeout time.Duration
	// HalfOpenMaxCalls is the max probe requests allowed in half-open state.
	HalfOpenMaxCalls int
}

// DefaultCircuitBreakerConfig returns sensible defaults: 5 failures, 30s reset, 1 probe.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern for external service calls.
// States: closed (normal) → open (rejecting) → half-open (probing) → closed.
type CircuitBreaker struct {
	mu               sync.Mutex
	name             string
	state            string
	failures         int
	successes        int // consecutive successes in half-open
	lastFailure      time.Time
	failureThreshold int
	resetTimeout     time.Duration
	halfOpenMax      int
	halfOpenCalls    int
	now              func() time.Time // for testing
}

// NewCircuitBreaker creates a circuit breaker with the given name and config.
func NewCircuitBreaker(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 1
	}
	return &CircuitBreaker{
		name:             name,
		state:            CircuitClosed,
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
		halfOpenMax:      cfg.HalfOpenMaxCalls,
		now:              time.Now,
	}
}

// Execute runs the given function through the circuit breaker.
// Returns ErrCircuitOpen if the circuit is open.
// If cb is nil, the function is executed directly without circuit breaking.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if cb == nil {
		return fn()
	}

	if err := cb.beforeCall(); err != nil {
		return err
	}

	err := fn()

	cb.afterCall(err)
	return err
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() string {
	if cb == nil {
		return CircuitClosed
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// Stats returns circuit breaker statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	if cb == nil {
		return CircuitBreakerStats{State: CircuitClosed}
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return CircuitBreakerStats{
		Name:        cb.name,
		State:       cb.currentState(),
		Failures:    cb.failures,
		LastFailure: cb.lastFailure,
	}
}

// CircuitBreakerStats holds circuit breaker metrics.
type CircuitBreakerStats struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Failures    int       `json:"failures"`
	LastFailure time.Time `json:"last_failure,omitempty"`
}

// currentState returns the effective state, accounting for timeout expiry.
// Must be called with mu held.
func (cb *CircuitBreaker) currentState() string {
	if cb.state == CircuitOpen {
		if cb.now().Sub(cb.lastFailure) >= cb.resetTimeout {
			return CircuitHalfOpen
		}
	}
	return cb.state
}

// beforeCall checks whether the call should be allowed.
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()
	switch state {
	case CircuitClosed:
		return nil
	case CircuitOpen:
		return fmt.Errorf("%s: %w", cb.name, ErrCircuitOpen)
	case CircuitHalfOpen:
		if cb.halfOpenCalls >= cb.halfOpenMax {
			return fmt.Errorf("%s: %w", cb.name, ErrCircuitOpen)
		}
		cb.halfOpenCalls++
		return nil
	}
	return nil
}

// afterCall records the result and transitions state.
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	if err == nil {
		cb.onSuccess(state)
	} else {
		// Don't count non-retryable errors (e.g., 4xx) as circuit failures
		var nre *noRetryError
		if errors.As(err, &nre) {
			return
		}
		cb.onFailure(state)
	}
}

func (cb *CircuitBreaker) onSuccess(state string) {
	switch state {
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			cb.reset()
		}
	case CircuitClosed:
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) onFailure(state string) {
	cb.failures++
	cb.lastFailure = cb.now()

	switch state {
	case CircuitClosed:
		if cb.failures >= cb.failureThreshold {
			cb.state = CircuitOpen
		}
	case CircuitHalfOpen:
		// Probe failed — back to open
		cb.state = CircuitOpen
		cb.halfOpenCalls = 0
		cb.successes = 0
	}
}

func (cb *CircuitBreaker) reset() {
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCalls = 0
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.reset()
}
