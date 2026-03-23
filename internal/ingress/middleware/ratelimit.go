package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter provides per-IP rate limiting using a token bucket algorithm.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int // requests per window
	window   time.Duration
}

type visitor struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a rate limiter allowing `rate` requests per `window`.
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	// Cleanup expired entries periodically
	go func() {
		for {
			time.Sleep(window * 2)
			rl.cleanup()
		}
	}()
	return rl
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
