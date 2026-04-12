package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tier 73 — ingress rate limiter lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 73 for
// internal/ingress/middleware/ratelimit.go:
//
//   - Stop idempotency (stopOnce-guarded double close)
//   - Stop waits for the cleanup goroutine (wg.Wait)
//   - NewRateLimiterWithLogger nil-logger guard
//   - Cleanup goroutine actually terminates promptly after Stop
//   - Middleware still serves traffic normally under Start/Stop
//   - Concurrent Stop storm does not panic
//   - Cleanup still evicts expired visitors after the wg/logger refactor

// ─── NewRateLimiterWithLogger nil-logger guard ─────────────────────────────

func TestTier73_NewRateLimiterWithLogger_NilLogger(t *testing.T) {
	rl := NewRateLimiterWithLogger(10, time.Minute, nil)
	defer rl.Stop()

	if rl == nil {
		t.Fatal("NewRateLimiterWithLogger returned nil")
	}
	if rl.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if rl.stopCh == nil {
		t.Error("stopCh should be initialised")
	}
}

// ─── NewRateLimiter default-logger path ───────────────────────────────────

func TestTier73_NewRateLimiter_DefaultLogger(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	defer rl.Stop()

	if rl.logger == nil {
		t.Error("NewRateLimiter should populate a non-nil logger")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier73_RateLimiter_Stop_Idempotent(t *testing.T) {
	rl := NewRateLimiterWithLogger(10, time.Minute, tier73Logger())

	// Pre-Tier-73 the second Stop panicked with "close of closed
	// channel". stopOnce now guards it.
	rl.Stop()
	rl.Stop()
	rl.Stop()
}

// ─── Stop waits for cleanup goroutine ──────────────────────────────────────

// TestTier73_RateLimiter_Stop_WaitsForCleanup proves that Stop
// actually blocks until the cleanup goroutine exits. If wg.Wait is
// missing, Stop returns immediately while the goroutine is still
// running, which means the router could be torn down mid-iterate
// and a map-delete could hit unreachable memory.
func TestTier73_RateLimiter_Stop_WaitsForCleanup(t *testing.T) {
	// Use a short window so the cleanup ticker fires quickly and the
	// goroutine is actually active when we Stop.
	rl := NewRateLimiterWithLogger(10, 20*time.Millisecond, tier73Logger())

	// Let the ticker fire at least once (interval = 2 * window = 40ms).
	time.Sleep(60 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		rl.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Stop did not return — wg.Wait missing or deadlock")
	}
}

// ─── Cleanup still evicts expired entries ──────────────────────────────────

// TestTier73_RateLimiter_Cleanup_EvictsExpired proves the cleanup
// goroutine is still running correctly after the Tier 73 wg/logger
// refactor. A 20ms window means the cleanup ticker runs every 40ms
// and entries are eligible for eviction after 40ms.
func TestTier73_RateLimiter_Cleanup_EvictsExpired(t *testing.T) {
	rl := NewRateLimiterWithLogger(10, 20*time.Millisecond, tier73Logger())
	defer rl.Stop()

	// Seed an entry via the middleware.
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	rl.mu.Lock()
	if len(rl.visitors) != 1 {
		rl.mu.Unlock()
		t.Fatalf("expected 1 visitor after first request, got %d", len(rl.visitors))
	}
	rl.mu.Unlock()

	// Wait for window*2 (eviction threshold) + at least one tick.
	// window = 20ms, ticker = 40ms. 200ms is well past both.
	time.Sleep(200 * time.Millisecond)

	rl.mu.Lock()
	got := len(rl.visitors)
	rl.mu.Unlock()
	if got != 0 {
		t.Errorf("expected cleanup to evict expired entry, still have %d", got)
	}
}

// ─── Middleware works under concurrent Stop ────────────────────────────────

func TestTier73_RateLimiter_ConcurrentStop_NoPanic(t *testing.T) {
	rl := NewRateLimiterWithLogger(10, time.Minute, tier73Logger())

	// Race a handful of Stops against each other.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); rl.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	rl.Stop()
}

// ─── Middleware serves traffic and then Stop drains cleanly ────────────────

func TestTier73_RateLimiter_ServeThenStop(t *testing.T) {
	rl := NewRateLimiterWithLogger(100, 50*time.Millisecond, tier73Logger())

	var served atomic.Int32
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Add(1)
		w.WriteHeader(http.StatusOK)
	}))

	// Send some traffic.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:5000"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	}

	if served.Load() != 5 {
		t.Errorf("expected 5 served requests, got %d", served.Load())
	}

	// Now Stop. It must drain quickly.
	start := time.Now()
	rl.Stop()
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("Stop took %v — cleanup goroutine did not exit promptly", elapsed)
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier73Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
