package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// GlobalRateLimiter enforces per-IP rate limits for all API endpoints.
// Uses an in-memory sliding window counter with automatic cleanup.
type GlobalRateLimiter struct {
	rate    int           // max requests per window
	window  time.Duration // window duration
	mu      sync.Mutex
	clients map[string]*rateLimitWindow
	stopCh  chan struct{}
}

type rateLimitWindow struct {
	count   int
	resetAt time.Time
}

// NewGlobalRateLimiter creates a rate limiter with the given rate per window.
// cleanup runs periodically to evict expired entries.
func NewGlobalRateLimiter(rate int, window time.Duration) *GlobalRateLimiter {
	rl := &GlobalRateLimiter{
		rate:    rate,
		window:  window,
		clients: make(map[string]*rateLimitWindow),
		stopCh:  make(chan struct{}),
	}

	// Background cleanup every 2x window duration
	go rl.cleanup(window * 2)

	return rl
}

// Middleware returns an HTTP middleware that enforces the rate limit.
func (rl *GlobalRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)
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

// Stop terminates the background cleanup goroutine.
func (rl *GlobalRateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *GlobalRateLimiter) cleanup(interval time.Duration) {
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
