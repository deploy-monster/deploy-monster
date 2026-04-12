package middleware

import (
	"sync"
	"time"
)

// WSFrameLimit default parameters.
//
// 100 frames/sec sustained is well above any legitimate control-plane
// traffic (pings are server-initiated, clients only send sparse JSON
// commands) while still letting a burst of 200 queued frames drain
// without forcing a disconnect. The defaults are package-level so
// tests and callers can override them before a hub is constructed.
const (
	WSFrameRatePerSec = 100
	WSFrameBurst      = 200
)

// WSFrameLimiter is a lock-free-ish per-connection token bucket for
// incoming WebSocket frames. Every call to Allow subtracts one token;
// a client that exceeds the configured rate is denied and the caller
// is expected to close the connection with a policy-violation code.
//
// Why a fresh bucket instead of golang.org/x/time/rate: rate.Limiter
// brings in a whole time-keeping abstraction we don't need, and the
// project rule is "minimum dependencies". A 40-line refill-on-check
// bucket is fine for ws flood control — it only runs on the receive
// path of active WebSocket connections, not in hot request handling.
//
// The zero value is NOT usable — always construct via NewWSFrameLimiter
// so rate and capacity default to sane values.
type WSFrameLimiter struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64 // tokens per second
	last     time.Time
	// now is overridable in tests so we don't have to block on a real
	// clock to verify refill math.
	now func() time.Time
}

// NewWSFrameLimiter returns a bucket seeded full. ratePerSec of 0 or
// less falls back to WSFrameRatePerSec; burst of 0 or less falls back
// to WSFrameBurst. Passing both as 0 yields the module-default bucket.
func NewWSFrameLimiter(ratePerSec, burst int) *WSFrameLimiter {
	if ratePerSec <= 0 {
		ratePerSec = WSFrameRatePerSec
	}
	if burst <= 0 {
		burst = WSFrameBurst
	}
	return &WSFrameLimiter{
		tokens:   float64(burst),
		capacity: float64(burst),
		rate:     float64(ratePerSec),
		now:      time.Now,
	}
}

// Allow consumes one token if the bucket is non-empty and returns
// true. A denied call leaves the bucket untouched so a sender that
// backs off naturally recovers without a penalty delay on top of the
// refill time.
//
// Refill happens lazily on every call: (elapsed seconds) * rate
// tokens are added, capped at capacity. This avoids spinning a
// goroutine per bucket and keeps the limiter state size to a single
// float64 + a timestamp.
func (l *WSFrameLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if l.last.IsZero() {
		l.last = now
	}
	elapsed := now.Sub(l.last).Seconds()
	if elapsed > 0 {
		l.tokens += elapsed * l.rate
		if l.tokens > l.capacity {
			l.tokens = l.capacity
		}
		l.last = now
	}

	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
