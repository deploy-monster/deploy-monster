package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewPersistentRateLimiter(t *testing.T) {
	rl := NewPersistentRateLimiter(nil, 10, time.Minute)

	if rl == nil {
		t.Fatal("expected non-nil PersistentRateLimiter")
	}
	if rl.rate != 10 {
		t.Errorf("expected rate 10, got %d", rl.rate)
	}
	if rl.window != time.Minute {
		t.Errorf("expected window 1m, got %v", rl.window)
	}
	if rl.bucket != "ratelimit" {
		t.Errorf("expected bucket 'ratelimit', got %q", rl.bucket)
	}
}

func TestPersistentRateLimiter_NilBolt_FallsBackToInMemory(t *testing.T) {
	// When bolt is nil, PersistentRateLimiter falls back to in-memory RateLimiter
	rl := NewPersistentRateLimiter(nil, 3, time.Minute)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	// First 3 requests should pass
	for i := range 3 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// 4th request should be rate limited
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after limit exceeded, got %d", rr.Code)
	}
}

func TestPersistentRateLimiter_NilBolt_DifferentIPs(t *testing.T) {
	rl := NewPersistentRateLimiter(nil, 1, time.Minute)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First IP
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "1.1.1.1:1000"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Error("first IP first request should pass")
	}

	// Second IP should also pass (separate rate limit)
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "2.2.2.2:2000"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Error("second IP first request should pass")
	}
}

func TestExtractIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	req.RemoteAddr = "10.0.0.1:1234"

	got := extractIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected '203.0.113.50', got %q", got)
	}
}

func TestExtractIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"

	got := extractIP(req)
	if got != "192.168.1.100" {
		t.Errorf("expected '192.168.1.100', got %q", got)
	}
}
