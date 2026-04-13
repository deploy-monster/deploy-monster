package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeout_CompletesBeforeDeadline(t *testing.T) {
	handler := Timeout(5 * time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify context has deadline
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("expected context to have deadline")
		}
		if time.Until(deadline) <= 0 {
			t.Error("expected deadline to be in the future")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTimeout_ContextHasDeadline(t *testing.T) {
	duration := 2 * time.Second
	handler := Timeout(duration)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("expected context to have deadline")
		}
		// The deadline should be within the specified duration (+/- some tolerance)
		remaining := time.Until(deadline)
		if remaining > duration {
			t.Errorf("deadline too far: %v, expected at most %v", remaining, duration)
		}
		if remaining < 0 {
			t.Error("deadline should not have already passed")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTimeout_ContextCancelledAfterHandler(t *testing.T) {
	// Short timeout — after ServeHTTP returns, the cancel is deferred.
	// We verify the context is not canceled during handler execution.
	handler := Timeout(10 * time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			t.Error("context should not be canceled during handler")
		default:
			// Good — context not yet canceled
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
