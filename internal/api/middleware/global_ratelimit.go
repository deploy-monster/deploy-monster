package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GlobalRateLimiter enforces per-IP rate limits for all API endpoints.
// Uses an in-memory sliding window counter with automatic cleanup.
//
// Lifecycle notes for Tier 72:
//
//   - Stop used to call close(stopCh) with no sync.Once guard, so a
//     second Stop crashed the whole API server with a "close of
//     closed channel" panic. stopOnce now serializes the close.
//   - The cleanup goroutine was spawned from NewGlobalRateLimiter
//     without any WaitGroup tracking, so Stop had no way to wait for
//     it to exit and the goroutine could outlive the router during
//     graceful shutdown. wg now tracks it.
//   - Stop was never actually called from api.Module.Stop, so the
//     cleanup goroutine leaked for the lifetime of the process on
//     every server restart during tests and dev runs. Module.Stop
//     now plumbs the call through.
//   - cleanup() had no defer/recover. A panic inside the map-iterate
//     loop (e.g. from a future enhancement that calls external code
//     under the lock) would crash the whole API server. The loop now
//     recovers and logs so the API stays up.
//   - NewGlobalRateLimiter tolerates a nil logger via
//     NewGlobalRateLimiterWithLogger by falling back to slog.Default,
//     matching the Tier 68/69/70/71 hardening style.
//
// Security notes for RATE-001:
//
//   - Only RemoteAddr is used for client identity. Attackers cannot
//     spoof XFF headers to bypass limits.
type GlobalRateLimiter struct {
	rate    int           // max requests per window
	window  time.Duration // window duration
	mu      sync.Mutex
	clients map[string]*rateLimitWindow
	stopCh  chan struct{}

	// Shutdown plumbing. wg tracks the cleanup goroutine so Stop can
	// wait for it to exit. stopOnce guards close(stopCh) against
	// double-Stop panics.
	stopOnce sync.Once
	wg       sync.WaitGroup
	logger   *slog.Logger

	// Optional path prefix allowlist. When empty (the default), every
	// request that reaches the middleware is rate-limited. When
	// populated, only requests whose URL path begins with one of the
	// listed prefixes are rate-limited — everything else passes
	// through untouched. The router sets this to {"/api/", "/hooks/"}
	// so that the embedded React SPA's static asset traffic does not
	// share a budget with API calls, which was the root cause of the
	// Tier 102 Playwright suite flapping red: 86 E2E tests loading the
	// login page × vite asset bundle trivially blew through the
	// default 120 req/min per-IP budget and served a JSON
	// "rate_limit exceeded" page in place of the login form.
	allowlist []string
}

type rateLimitWindow struct {
	count   int
	resetAt time.Time
}

// NewGlobalRateLimiter creates a rate limiter with the given rate
// per window. A rate of 0 or less disables limiting. The cleanup
// goroutine runs every 2x window to evict
// expired entries. Logs go to slog.Default() — production callers
// should prefer NewGlobalRateLimiterWithLogger so panic logs are
// tagged with the api module.
func NewGlobalRateLimiter(rate int, window time.Duration) *GlobalRateLimiter {
	return NewGlobalRateLimiterWithLogger(rate, window, nil)
}

// NewGlobalRateLimiterWithLogger creates a rate limiter bound to a
// structured logger. A nil logger is tolerated and replaced with
// slog.Default().
func NewGlobalRateLimiterWithLogger(rate int, window time.Duration, logger *slog.Logger) *GlobalRateLimiter {
	if logger == nil {
		logger = slog.Default()
	}
	rl := &GlobalRateLimiter{
		rate:    rate,
		window:  window,
		clients: make(map[string]*rateLimitWindow),
		stopCh:  make(chan struct{}),
		logger:  logger,
	}

	// Background cleanup every 2x window duration. Tracked by wg so
	// Stop can wait for it to exit before the router is torn down.
	rl.wg.Add(1)
	go rl.cleanup(window * 2)

	return rl
}

// SetRateLimitedPrefixes installs a path-prefix allowlist. Only
// requests whose URL path begins with one of the listed prefixes will
// be subject to rate limiting; everything else (SPA assets, health,
// static bundle) passes through. Call with nil or an empty slice to
// restore the default "rate-limit everything" behavior. Safe to call
// once at router-construction time; not safe for concurrent mutation
// after traffic starts.
func (rl *GlobalRateLimiter) SetRateLimitedPrefixes(prefixes []string) {
	if len(prefixes) == 0 {
		rl.allowlist = nil
		return
	}
	copied := make([]string, len(prefixes))
	copy(copied, prefixes)
	rl.allowlist = copied
}

// shouldRateLimit returns true if the request path falls inside the
// configured allowlist, or if no allowlist is set (legacy behavior).
func (rl *GlobalRateLimiter) shouldRateLimit(path string) bool {
	if len(rl.allowlist) == 0 {
		return true
	}
	for _, prefix := range rl.allowlist {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// Middleware returns an HTTP middleware that enforces the rate limit.
func (rl *GlobalRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.rate <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		if !rl.shouldRateLimit(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ip := safeClientIP(r, false)
		now := time.Now()

		rl.mu.Lock()
		entry, exists := rl.clients[ip]
		if !exists || now.After(entry.resetAt) {
			// New window
			rl.clients[ip] = &rateLimitWindow{
				count:   1,
				resetAt: now.Add(rl.window),
			}
			rl.mu.Unlock()
			rl.setRateLimitHeaders(w, rl.rate-1, now.Add(rl.window))
			next.ServeHTTP(w, r)
			return
		}

		if entry.count >= rl.rate {
			retryAfter := int(time.Until(entry.resetAt).Seconds()) + 1
			rl.mu.Unlock()
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			rl.setRateLimitHeaders(w, 0, entry.resetAt)
			writeErrorJSON(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		entry.count++
		remaining := rl.rate - entry.count
		resetAt := entry.resetAt
		rl.mu.Unlock()

		rl.setRateLimitHeaders(w, remaining, resetAt)
		next.ServeHTTP(w, r)
	})
}

func (rl *GlobalRateLimiter) setRateLimitHeaders(w http.ResponseWriter, remaining int, resetAt time.Time) {
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.rate))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))
}

// RateLimitStats holds point-in-time rate limiter statistics.
type RateLimitStats struct {
	Rate          int `json:"rate_per_window"`
	ActiveClients int `json:"active_clients"`
}

// Stats returns current rate limiter statistics.
func (rl *GlobalRateLimiter) Stats() RateLimitStats {
	rl.mu.Lock()
	n := len(rl.clients)
	rl.mu.Unlock()
	return RateLimitStats{
		Rate:          rl.rate,
		ActiveClients: n,
	}
}

// Stop terminates the background cleanup goroutine. Safe to call
// multiple times; the second and subsequent calls are no-ops. Stop
// blocks until the cleanup goroutine exits so the caller can rely
// on "after Stop returns, no more map mutations will happen".
func (rl *GlobalRateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		if rl.stopCh != nil {
			close(rl.stopCh)
		}
	})
	rl.wg.Wait()
}

func (rl *GlobalRateLimiter) cleanup(interval time.Duration) {
	defer rl.wg.Done()
	defer func() {
		// Pre-Tier-72 this goroutine had no recover. A panic anywhere
		// inside the map iterate (which holds the mu lock) would crash
		// the entire API server. Recover keeps the API alive even if
		// a future enhancement accidentally panics under the lock.
		if r := recover(); r != nil {
			if rl.logger != nil {
				rl.logger.Error("panic in global rate limiter cleanup", "error", r)
			}
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case now := <-ticker.C:
			rl.mu.Lock()
			for ip, entry := range rl.clients {
				if now.After(entry.resetAt) {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}
