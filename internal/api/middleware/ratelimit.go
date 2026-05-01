package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AuthRateLimiter enforces per-IP rate limits on auth endpoints using BBolt.
// SECURITY FIX: Uses mutex to prevent race conditions on read-modify-write operations.
type AuthRateLimiter struct {
	bolt   core.BoltStorer
	rate   int
	window time.Duration
	prefix string
	logger *slog.Logger
	mu     sync.Mutex // protects bolt operations for same key
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

// safeClientIP returns the real client IP, respecting the trustXFF setting.
// When trustXFF is false (default), only r.RemoteAddr is used.
// When trustXFF is true, X-Real-IP and X-Forwarded-For are used if available,
// but XFF is validated to prevent spoofing attacks.
func safeClientIP(r *http.Request, trustXFF bool) string {
	if !trustXFF {
		return stripPort(r.RemoteAddr)
	}

	// X-Real-IP takes priority (set by nginx Real IP module)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		if validated := validateIP(ip); validated != "" {
			return validated
		}
	}

	// X-Forwarded-For: first IP in the chain (closest proxy to client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF can be "client, proxy1, proxy2" — take first (client)
		first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if validated := validateIP(first); validated != "" {
			return validated
		}
	}

	return stripPort(r.RemoteAddr)
}

// validateIP returns the IP string if it is a valid public IP, else empty string.
// This prevents arbitrary byte injection via crafted XFF values.
func validateIP(raw string) string {
	if raw == "" {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return ""
	}
	// Reject private, loopback, link-local IPs in rate limit context
	// to prevent attackers from using internal IPs to bypass limits.
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return ""
	}
	return ip.String()
}

// stripPort removes the :port suffix from RemoteAddr (e.g., "1.2.3.4:8080" -> "1.2.3.4").
func stripPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		// Check if it looks like an IPv6 address (contains multiple colons)
		if !strings.Contains(addr, "]") && strings.Count(addr, ":") == 1 {
			return addr[:i]
		}
	}
	return addr
}

type authRateLimitEntry struct {
	Count   int   `json:"c"`
	ResetAt int64 `json:"r"`
}

// Wrap returns a handler function that enforces the rate limit before calling next.
// SECURITY FIX: Uses mutex to prevent TOCTOU race condition on rate limit counter.
func (rl *AuthRateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	if rl.bolt == nil {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ip := safeClientIP(r, false)
		key := fmt.Sprintf("auth_rl:%s:%s", rl.prefix, ip)

		// Acquire lock to prevent race condition
		rl.mu.Lock()
		defer rl.mu.Unlock()

		var entry authRateLimitEntry
		now := time.Now().Unix()

		err := rl.bolt.Get("ratelimit", key, &entry)
		if err != nil || now >= entry.ResetAt {
			// New window
			entry = authRateLimitEntry{
				Count:   1,
				ResetAt: now + int64(rl.window.Seconds()),
			}
			if err := rl.bolt.Set("ratelimit", key, entry, int64(rl.window.Seconds())); err != nil && rl.logger != nil {
				rl.logger.Error("auth ratelimit set failed", "key", key, "error", err)
			}
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
		if err := rl.bolt.Set("ratelimit", key, entry, int64(rl.window.Seconds())); err != nil && rl.logger != nil {
			rl.logger.Error("auth ratelimit increment failed", "key", key, "error", err)
		}

		next.ServeHTTP(w, r)
	}
}
