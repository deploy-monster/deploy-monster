package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGracefulShutdown_Normal(t *testing.T) {
	gs := NewGracefulShutdown()
	handler := gs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// During request, inFlight should be 1
		if gs.InFlight() != 1 {
			t.Errorf("expected 1 in-flight, got %d", gs.InFlight())
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// After request, inFlight should be 0
	if gs.InFlight() != 0 {
		t.Errorf("expected 0 in-flight after request, got %d", gs.InFlight())
	}
}

func TestGracefulShutdown_Draining(t *testing.T) {
	gs := NewGracefulShutdown()
	gs.StartDraining()

	handler := gs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach handler during drain")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 during drain, got %d", rr.Code)
	}

	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestBodyLimit(t *testing.T) {
	handler := BodyLimit(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
