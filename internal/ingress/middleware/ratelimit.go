package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter provides per-IP rate limiting using a token bucket algorithm.
//
// Lifecycle notes for Tier 73:
//
//   - Stop used to call close(stopCh) with no sync.Once guard. A
//     double Stop closed an already-closed channel and crashed the
//     ingress gateway — which sits on the public edge for every
//     tenant's traffic. stopOnce now serializes the close.
//   - The cleanup goroutine was spawned from NewRateLimiter with no
//     WaitGroup tracking, so Stop had no way to wait for it to exit
//     and could easily return while map deletes were still in flight.
//     wg now tracks it.
//   - cleanup() had no defer/recover. A panic anywhere inside the
//     map-iterate loop (which holds the mu lock) would crash the
//     ingress gateway. The loop now recovers and logs.
//   - NewRateLimiter tolerates a nil logger via NewRateLimiterWithLogger
//     by falling back to slog.Default, matching the Tier 68/69/70/71/72
//     hardening style.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int // requests per window
	window   time.Duration
	stopCh   chan struct{}

	// Shutdown plumbing. wg tracks the cleanup goroutine so Stop can
	// wait for it to exit. stopOnce guards close(stopCh) against
	// double-Stop panics.
	stopOnce sync.Once
	wg       sync.WaitGroup
	logger   *slog.Logger
}

type visitor struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a rate limiter allowing `rate` requests per
// `window`. Logs go to slog.Default() — production callers should
// prefer NewRateLimiterWithLogger so panic logs are tagged with the
// ingress module.
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	return NewRateLimiterWithLogger(rate, window, nil)
}

// NewRateLimiterWithLogger creates a rate limiter bound to a
// structured logger. A nil logger is tolerated and replaced with
// slog.Default().
func NewRateLimiterWithLogger(rate int, window time.Duration, logger *slog.Logger) *RateLimiter {
	if logger == nil {
		logger = slog.Default()
	}
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
	rl.wg.Add(1)
	go rl.cleanupLoop(window * 2)
	return rl
}

// Stop halts the background cleanup goroutine. Safe to call multiple
// times; the second and subsequent calls are no-ops. Stop blocks until
// the cleanup goroutine exits so the caller can rely on "after Stop
// returns, no more map mutations will happen".
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		if rl.stopCh != nil {
			close(rl.stopCh)
		}
	})
	rl.wg.Wait()
}

// Middleware returns an HTTP middleware that enforces the rate limit.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[ip]
	if !ok || time.Since(v.lastReset) > rl.window {
		rl.visitors[ip] = &visitor{tokens: rl.rate - 1, lastReset: time.Now()}
		return true
	}

	if v.tokens <= 0 {
		return false
	}

	v.tokens--
	return true
}

// cleanupLoop runs the expired-visitor eviction ticker. Separated
// from the anonymous goroutine closure so the defer/recover + wg.Done
// bookkeeping is easy to audit.
func (rl *RateLimiter) cleanupLoop(interval time.Duration) {
	defer rl.wg.Done()
	defer func() {
		// Pre-Tier-73 this goroutine had no recover. A panic anywhere
		// inside cleanup() (which runs under the global mu lock) would
		// crash the ingress gateway for every tenant. Recover keeps
		// the gateway alive even if a future enhancement accidentally
		// panics under the lock.
		if r := recover(); r != nil {
			if rl.logger != nil {
				rl.logger.Error("panic in ingress rate limiter cleanup", "error", r)
			}
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, v := range rl.visitors {
		if time.Since(v.lastReset) > rl.window*2 {
			delete(rl.visitors, ip)
		}
	}
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.SplitN(xff, ",", 2)[0]
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}
