package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// tenantRateLimitConfig mirrors the handler-side config stored in BBolt.
type tenantRateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstSize         int `json:"burst_size"`
}

// TenantRateLimiter enforces per-tenant API rate limits using BBolt storage.
// It reads each tenant's configured limits from the "tenant_ratelimit" bucket
// and enforces them with an in-memory sliding window per tenant.
// SECURITY FIX (RACE-002): Uses sync.Map for thread-safe tenant tracking.
type TenantRateLimiter struct {
	bolt          core.BoltStorer
	defaultRate   int
	defaultWindow time.Duration
	// SECURITY FIX (RACE-002): Use sync.Map for thread-safe access to tenant entries
	entries sync.Map   // map[string]*tenantRateLimitEntry
	mu      sync.Mutex // protects bolt operations for same key
}

type tenantRateLimitEntry struct {
	Count   int   `json:"c"`
	ResetAt int64 `json:"r"`
}

// NewTenantRateLimiter creates a tenant-aware rate limiter.
// defaultRate is used when no per-tenant config exists in BBolt.
func NewTenantRateLimiter(bolt core.BoltStorer, defaultRate int, window time.Duration) *TenantRateLimiter {
	return &TenantRateLimiter{
		bolt:          bolt,
		defaultRate:   defaultRate,
		defaultWindow: window,
	}
}

// Middleware returns an HTTP middleware that enforces per-tenant rate limits.
// Must be applied AFTER RequireAuth so claims are present in context.
// SECURITY FIX (RACE-002): Uses mutex for thread-safe operations.
func (trl *TenantRateLimiter) Middleware(next http.Handler) http.Handler {
	if trl.bolt == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil || claims.TenantID == "" {
			// No tenant context — skip (unauthenticated or system request)
			next.ServeHTTP(w, r)
			return
		}

		tenantID := claims.TenantID

		// Read tenant-specific config from BBolt (fast KV lookup)
		rate := trl.defaultRate
		var cfg tenantRateLimitConfig
		if err := trl.bolt.Get("tenant_ratelimit", tenantID, &cfg); err == nil && cfg.RequestsPerMinute > 0 {
			rate = cfg.RequestsPerMinute
		}

		// Enforce sliding window per tenant
		key := fmt.Sprintf("trl:%s", tenantID)
		now := time.Now().Unix()
		windowSec := int64(trl.defaultWindow.Seconds())

		// SECURITY FIX (RACE-002): Lock for thread-safe read-modify-write
		trl.mu.Lock()
		defer trl.mu.Unlock()

		var entry tenantRateLimitEntry
		err := trl.bolt.Get("ratelimit", key, &entry)
		if err != nil || now >= entry.ResetAt {
			// New window
			entry = tenantRateLimitEntry{
				Count:   1,
				ResetAt: now + windowSec,
			}
			_ = trl.bolt.Set("ratelimit", key, entry, windowSec)
			setTenantRateLimitHeaders(w, rate, rate-1, time.Unix(entry.ResetAt, 0))
			next.ServeHTTP(w, r)
			return
		}

		if entry.Count >= rate {
			retryAfter := entry.ResetAt - now
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			setTenantRateLimitHeaders(w, rate, 0, time.Unix(entry.ResetAt, 0))
			writeErrorJSON(w, http.StatusTooManyRequests, "tenant rate limit exceeded")
			return
		}

		entry.Count++
		remaining := rate - entry.Count
		_ = trl.bolt.Set("ratelimit", key, entry, windowSec)

		setTenantRateLimitHeaders(w, rate, remaining, time.Unix(entry.ResetAt, 0))
		next.ServeHTTP(w, r)
	})
}

func setTenantRateLimitHeaders(w http.ResponseWriter, limit, remaining int, resetAt time.Time) {
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))
}
