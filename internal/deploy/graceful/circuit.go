package graceful

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed means requests flow normally.
	CircuitClosed CircuitState = iota
	// CircuitOpen means all requests fail fast.
	CircuitOpen
	// CircuitHalfOpen means limited requests are allowed to test recovery.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitConfig holds configuration for the circuit breaker.
type CircuitConfig struct {
	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of successes in half-open to close.
	SuccessThreshold int
	// Timeout is how long to wait before trying half-open state.
	Timeout time.Duration
}

// DefaultCircuitConfig returns sensible defaults.
func DefaultCircuitConfig() CircuitConfig {
	return CircuitConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreaker implements the circuit breaker pattern for a single backend.
type CircuitBreaker struct {
	mu sync.RWMutex

	state           CircuitState
	failureCount    int
	successCount    int
	lastFailTime    time.Time
	lastStateChange time.Time

	config CircuitConfig
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:           CircuitClosed,
		config:          config,
		lastStateChange: time.Now(),
	}
}

// AllowRequest checks if a request should be allowed.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailTime) > cb.config.Timeout {
			cb.state = CircuitHalfOpen
			cb.successCount = 0
			cb.lastStateChange = time.Now()
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		// Reset failure count on success
		cb.failureCount = 0
	case CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.state = CircuitClosed
			cb.failureCount = 0
			cb.successCount = 0
			cb.lastStateChange = time.Now()
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now()
		}
	case CircuitHalfOpen:
		// Any failure in half-open goes back to open
		cb.state = CircuitOpen
		cb.successCount = 0
		cb.lastStateChange = time.Now()
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset forces the circuit to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastStateChange = time.Now()
}

// Stats returns current circuit breaker statistics.
func (cb *CircuitBreaker) Stats() CircuitStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitStats{
		State:           cb.state,
		FailureCount:    cb.failureCount,
		SuccessCount:    cb.successCount,
		LastFailTime:    cb.lastFailTime,
		LastStateChange: cb.lastStateChange,
	}
}

// CircuitStats holds circuit breaker statistics.
type CircuitStats struct {
	State           CircuitState
	FailureCount    int
	SuccessCount    int
	LastFailTime    time.Time
	LastStateChange time.Time
}

// CircuitBreakerManager manages circuit breakers for multiple backends.
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitConfig
}

// NewCircuitBreakerManager creates a new manager.
func NewCircuitBreakerManager(config CircuitConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// AllowRequest checks if a request to the backend should be allowed.
func (m *CircuitBreakerManager) AllowRequest(backend string) bool {
	m.mu.RLock()
	cb, ok := m.breakers[backend]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		cb, ok = m.breakers[backend]
		if !ok {
			cb = NewCircuitBreaker(m.config)
			m.breakers[backend] = cb
		}
		m.mu.Unlock()
	}

	return cb.AllowRequest()
}

// RecordSuccess records a successful request to the backend.
func (m *CircuitBreakerManager) RecordSuccess(backend string) {
	m.mu.RLock()
	cb, ok := m.breakers[backend]
	m.mu.RUnlock()

	if ok {
		cb.RecordSuccess()
	}
}

// RecordFailure records a failed request to the backend.
func (m *CircuitBreakerManager) RecordFailure(backend string) {
	m.mu.RLock()
	cb, ok := m.breakers[backend]
	m.mu.RUnlock()

	if ok {
		cb.RecordFailure()
		return
	}

	// Create new breaker and record failure
	m.mu.Lock()
	cb, ok = m.breakers[backend]
	if !ok {
		cb = NewCircuitBreaker(m.config)
		m.breakers[backend] = cb
	}
	m.mu.Unlock()

	cb.RecordFailure()
}

// State returns the circuit state for a backend.
func (m *CircuitBreakerManager) State(backend string) CircuitState {
	m.mu.RLock()
	cb, ok := m.breakers[backend]
	m.mu.RUnlock()

	if !ok {
		return CircuitClosed
	}
	return cb.State()
}

// Reset resets the circuit breaker for a backend.
func (m *CircuitBreakerManager) Reset(backend string) {
	m.mu.RLock()
	cb, ok := m.breakers[backend]
	m.mu.RUnlock()

	if ok {
		cb.Reset()
	}
}

// Stats returns stats for a backend's circuit breaker.
func (m *CircuitBreakerManager) Stats(backend string) (CircuitStats, bool) {
	m.mu.RLock()
	cb, ok := m.breakers[backend]
	m.mu.RUnlock()

	if !ok {
		return CircuitStats{}, false
	}
	return cb.Stats(), true
}

// AllStats returns stats for all backends.
func (m *CircuitBreakerManager) AllStats() map[string]CircuitStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]CircuitStats, len(m.breakers))
	for backend, cb := range m.breakers {
		result[backend] = cb.Stats()
	}
	return result
}

// IsOpen returns true if the circuit is open (failing fast).
func (m *CircuitBreakerManager) IsOpen(backend string) bool {
	return m.State(backend) == CircuitOpen
}

// IsAvailable returns true if requests can be made to the backend.
func (m *CircuitBreakerManager) IsAvailable(backend string) bool {
	return m.AllowRequest(backend)
}

var (
	// ErrCircuitOpen is returned when the circuit is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
)
