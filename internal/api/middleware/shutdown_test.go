package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
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

func TestGracefulShutdown_DrainWaitsForInFlight(t *testing.T) {
	gs := NewGracefulShutdown()

	// Start a long-running request
	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})

	handler := gs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		wg.Wait() // Block until we release
		w.WriteHeader(http.StatusOK)
	}))

	go func() {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}()

	<-started // Wait for request to be in-flight

	if gs.InFlight() != 1 {
		t.Fatalf("expected 1 in-flight, got %d", gs.InFlight())
	}

	// Start draining — new requests rejected
	gs.StartDraining()

	// Simulate the drain wait loop from module.go Stop()
	drained := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for gs.InFlight() > 0 {
			<-ticker.C
		}
		close(drained)
	}()

	// Should NOT be drained yet
	select {
	case <-drained:
		t.Fatal("should not be drained while request is in-flight")
	case <-time.After(50 * time.Millisecond):
	}

	// Release the request
	wg.Done()

	// Should drain within a reasonable time
	select {
	case <-drained:
		// OK
	case <-time.After(time.Second):
		t.Fatal("drain did not complete after request finished")
	}

	if gs.InFlight() != 0 {
		t.Errorf("expected 0 in-flight after drain, got %d", gs.InFlight())
	}
}

func TestGracefulShutdown_IsDrainingFlag(t *testing.T) {
	gs := NewGracefulShutdown()

	if gs.IsDraining() {
		t.Error("should not be draining initially")
	}

	gs.StartDraining()

	if !gs.IsDraining() {
		t.Error("should be draining after StartDraining")
	}
}

func TestGracefulShutdown_ConcurrentRequests(t *testing.T) {
	gs := NewGracefulShutdown()
	const n = 10

	started := make(chan struct{})
	release := make(chan struct{})

	handler := gs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	// Launch n concurrent requests
	for i := 0; i < n; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}()
	}

	// Wait for all to start
	for i := 0; i < n; i++ {
		<-started
	}

	if gs.InFlight() != int64(n) {
		t.Errorf("expected %d in-flight, got %d", n, gs.InFlight())
	}

	// Release all
	close(release)

	// Wait for drain
	time.Sleep(50 * time.Millisecond)
	if gs.InFlight() != 0 {
		t.Errorf("expected 0 in-flight after release, got %d", gs.InFlight())
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
