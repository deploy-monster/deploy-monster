package ingress

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/deploy/graceful"
)

func TestResponseTracker_Write_RecordsSuccess(t *testing.T) {
	backend := httptest.NewRecorder()
	cb := graceful.NewCircuitBreakerManager(graceful.DefaultCircuitConfig())
	// Pre-seed the circuit breaker so RecordSuccess has something to update.
	cb.AllowRequest("127.0.0.1:3000")
	cb.RecordFailure("127.0.0.1:3000")

	rt := &responseTracker{
		ResponseWriter: backend,
		backend:        "127.0.0.1:3000",
		circuit:        cb,
	}

	n, err := rt.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Errorf("n = %d, want 5", n)
	}
	if !rt.written {
		t.Error("written should be true")
	}
	if rt.status != 200 {
		t.Errorf("status = %d, want 200", rt.status)
	}

	// Success should reset the failure count
	stats, ok := cb.Stats("127.0.0.1:3000")
	if !ok {
		t.Fatal("expected circuit stats")
	}
	if stats.FailureCount != 0 {
		t.Errorf("failure count = %d, want 0", stats.FailureCount)
	}
}

func TestResponseTracker_Write_AlreadyWritten(t *testing.T) {
	backend := httptest.NewRecorder()
	cb := graceful.NewCircuitBreakerManager(graceful.DefaultCircuitConfig())
	rt := &responseTracker{
		ResponseWriter: backend,
		backend:        "127.0.0.1:3000",
		circuit:        cb,
		written:        true,
		status:         500,
	}

	n, err := rt.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Errorf("n = %d, want 5", n)
	}
	// Status should remain unchanged
	if rt.status != 500 {
		t.Errorf("status = %d, want 500", rt.status)
	}
}

func TestResponseTracker_WriteHeader_RecordsFailure(t *testing.T) {
	backend := httptest.NewRecorder()
	cb := graceful.NewCircuitBreakerManager(graceful.DefaultCircuitConfig())
	rt := &responseTracker{
		ResponseWriter: backend,
		backend:        "127.0.0.1:3000",
		circuit:        cb,
	}

	rt.WriteHeader(http.StatusInternalServerError)
	if rt.status != 500 {
		t.Errorf("status = %d, want 500", rt.status)
	}
	if !rt.written {
		t.Error("written should be true")
	}

	stats, ok := cb.Stats("127.0.0.1:3000")
	if !ok {
		t.Fatal("expected circuit stats")
	}
	if stats.FailureCount != 1 {
		t.Errorf("failure count = %d, want 1", stats.FailureCount)
	}
}

func TestResponseTracker_WriteHeader_AlreadyWritten(t *testing.T) {
	backend := httptest.NewRecorder()
	cb := graceful.NewCircuitBreakerManager(graceful.DefaultCircuitConfig())
	rt := &responseTracker{
		ResponseWriter: backend,
		backend:        "127.0.0.1:3000",
		circuit:        cb,
		written:        true,
		status:         200,
	}

	// Pre-record a success
	cb.RecordSuccess("127.0.0.1:3000")

	rt.WriteHeader(http.StatusInternalServerError)
	// Status should remain 200 because written was already true
	if rt.status != 200 {
		t.Errorf("status = %d, want 200 (already written)", rt.status)
	}

	// No additional failure should be recorded
	stats, _ := cb.Stats("127.0.0.1:3000")
	if stats.FailureCount != 0 {
		t.Errorf("failure count = %d, want 0 (header ignored)", stats.FailureCount)
	}
}
