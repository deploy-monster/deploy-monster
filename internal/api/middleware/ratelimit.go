package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AuthRateLimiter enforces per-IP rate limits on auth endpoints using BBolt.
type AuthRateLimiter struct {
	bolt   core.BoltStorer
	rate   int
	window time.Duration
	prefix string
}

type authRateLimitEntry struct {
	Count   int   `json:"c"`
	ResetAt int64 `json:"r"`
}

// NewAuthRateLimiter creates a rate limiter for auth endpoints.
// rate is the max number of requests allowed per window per IP.
// prefix differentiates keys (e.g., "login", "register").
func NewAuthRateLimiter(bolt core.BoltStorer, rate int, window time.Duration, prefix string) *AuthRateLimiter {
	return &AuthRateLimiter{
		bolt:   bolt,
		rate:   rate,
		window: window,
		prefix: prefix,
	}
}

// Wrap returns a handler function that enforces the rate limit before calling next.
func (rl *AuthRateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	if rl.bolt == nil {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)
		key := fmt.Sprintf("auth_rl:%s:%s", rl.prefix, ip)

		var entry authRateLimitEntry
		now := time.Now().Unix()

		err := rl.bolt.Get("ratelimit", key, &entry)
		if err != nil || now >= entry.ResetAt {
			// New window
			entry = authRateLimitEntry{
				Count:   1,
				ResetAt: now + int64(rl.window.Seconds()),
			}
			_ = rl.bolt.Set("ratelimit", key, entry, int64(rl.window.Seconds()))
			next.ServeHTTP(w, r)
			return
		}

		if entry.Count >= rl.rate {
			retryAfter := entry.ResetAt - now
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.rate))
			w.Header().Set("X-RateLimit-Remaining", "0")
			writeErrorJSON(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		entry.Count++
		_ = rl.bolt.Set("ratelimit", key, entry, int64(rl.window.Seconds()))

		next.ServeHTTP(w, r)
	}
}
