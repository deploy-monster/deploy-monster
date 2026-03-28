package middleware

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PersistentRateLimiter uses BBolt for rate limit state that survives restarts.
// Falls back to in-memory if BBolt is not available.
type PersistentRateLimiter struct {
	bolt   core.BoltStorer
	rate   int
	window time.Duration
	bucket string
}

type rateLimitEntry struct {
	Count   int   `json:"c"`
	ResetAt int64 `json:"r"`
}

// NewPersistentRateLimiter creates a rate limiter backed by BBolt.
func NewPersistentRateLimiter(bolt core.BoltStorer, rate int, window time.Duration) *PersistentRateLimiter {
	return &PersistentRateLimiter{
		bolt:   bolt,
		rate:   rate,
		window: window,
		bucket: "ratelimit",
	}
}

// Middleware returns HTTP middleware that enforces persistent rate limits.
func (rl *PersistentRateLimiter) Middleware(next http.Handler) http.Handler {
	// Fall back to in-memory if bolt not available
	if rl.bolt == nil {
		mem := NewRateLimiter(rl.rate, rl.window)
		return mem.Middleware(next)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		key := "rl:" + ip

		var entry rateLimitEntry
		err := rl.bolt.Get(rl.bucket, key, &entry)

		now := time.Now().Unix()

		if err != nil || now >= entry.ResetAt {
			// New window
			entry = rateLimitEntry{
				Count:   1,
				ResetAt: now + int64(rl.window.Seconds()),
			}
			_ = rl.bolt.Set(rl.bucket, key, entry, int64(rl.window.Seconds()))
			next.ServeHTTP(w, r)
			return
		}

		if entry.Count >= rl.rate {
			retryAfter := entry.ResetAt - now
			w.Header().Set("Retry-After", json.Number(string(rune(retryAfter))).String())
			w.Header().Set("X-RateLimit-Limit", json.Number(string(rune(rl.rate))).String())
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		entry.Count++
		_ = rl.bolt.Set(rl.bucket, key, entry, int64(rl.window.Seconds()))

		next.ServeHTTP(w, r)
	})
}
