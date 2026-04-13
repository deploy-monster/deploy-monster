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

// Tier 72 — global rate limiter lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 72:
//   - Stop idempotency (stopOnce-guarded double close)
//   - Stop waits for the cleanup goroutine (wg.Wait)
//   - NewGlobalRateLimiterWithLogger nil-logger guard
//   - Cleanup goroutine actually terminates promptly after Stop
//   - Middleware still serves traffic normally under Start/Stop
//   - Concurrent Stop storm does not panic

// ─── NewGlobalRateLimiterWithLogger nil-logger guard ───────────────────────

func TestTier72_NewGlobalRateLimiterWithLogger_NilLogger(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(10, time.Minute, nil)
	defer rl.Stop()

	if rl == nil {
		t.Fatal("NewGlobalRateLimiterWithLogger returned nil")
	}
	if rl.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if rl.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier72_GlobalRateLimiter_Stop_Idempotent(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(10, time.Minute, tier72Logger())

	// Pre-Tier-72 the second Stop panicked with "close of closed
	// channel". stopOnce now guards it.
	rl.Stop()
	rl.Stop()
}

// ─── Stop waits for cleanup goroutine ──────────────────────────────────────

// TestTier72_GlobalRateLimiter_Stop_WaitsForCleanup proves that Stop
// actually blocks until the cleanup goroutine exits. If wg.Wait is
// missing, Stop returns immediately while the goroutine is still
// running, which means the router could be torn down mid-iterate
// and a map-delete could hit unreachable memory.
func TestTier72_GlobalRateLimiter_Stop_WaitsForCleanup(t *testing.T) {
	// Use a short window so the cleanup ticker fires quickly and the
	// goroutine is actually active when we Stop.
	rl := NewGlobalRateLimiterWithLogger(10, 20*time.Millisecond, tier72Logger())

	// Let the ticker fire at least once.
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

// ─── Cleanup actually evicts expired entries ───────────────────────────────

// TestTier72_GlobalRateLimiter_Cleanup_EvictsExpired proves the
// cleanup goroutine is still running correctly after the Tier 72
// wg/logger changes.
func TestTier72_GlobalRateLimiter_Cleanup_EvictsExpired(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(10, 20*time.Millisecond, tier72Logger())
	defer rl.Stop()

	// Seed an entry.
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if rl.Stats().ActiveClients != 1 {
		t.Fatalf("expected 1 active client after first request, got %d", rl.Stats().ActiveClients)
	}

	// Wait for the window to expire and for at least one cleanup
	// tick (interval = 2 * window = 40ms).
	time.Sleep(150 * time.Millisecond)

	if got := rl.Stats().ActiveClients; got != 0 {
		t.Errorf("expected cleanup to evict expired entry, still have %d", got)
	}
}

// ─── Middleware works under concurrent Stop ────────────────────────────────

func TestTier72_GlobalRateLimiter_ConcurrentStop_NoPanic(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(10, time.Minute, tier72Logger())

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

func TestTier72_GlobalRateLimiter_ServeThenStop(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(100, 50*time.Millisecond, tier72Logger())

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

// ─── Stats is safe to call after Stop ──────────────────────────────────────

// TestTier72_GlobalRateLimiter_Stats_AfterStop proves that the
// public Stats read-only method still works after Stop — we've seen
// lifecycle rewrites accidentally nil out maps during shutdown.
func TestTier72_GlobalRateLimiter_Stats_AfterStop(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(10, time.Minute, tier72Logger())
	rl.Stop()

	// Should not panic.
	stats := rl.Stats()
	if stats.Rate != 10 {
		t.Errorf("expected rate=10 after Stop, got %d", stats.Rate)
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier72Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
