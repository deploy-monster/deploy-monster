package middleware

import (
	"net/http"
	"sync/atomic"
)

// GracefulShutdown tracks in-flight requests for clean shutdown.
type GracefulShutdown struct {
	inFlight atomic.Int64
	draining atomic.Bool
}

// NewGracefulShutdown creates a shutdown tracker.
func NewGracefulShutdown() *GracefulShutdown {
	return &GracefulShutdown{}
}

// Middleware tracks in-flight requests and rejects new ones during drain.
func (gs *GracefulShutdown) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gs.draining.Load() {
			w.Header().Set("Connection", "close")
			w.Header().Set("Retry-After", "5")
			http.Error(w, `{"error":"server is shutting down"}`, http.StatusServiceUnavailable)
			return
		}

		gs.inFlight.Add(1)
		defer gs.inFlight.Add(-1)

		next.ServeHTTP(w, r)
	})
}

// StartDraining signals that no new requests should be accepted.
func (gs *GracefulShutdown) StartDraining() {
	gs.draining.Store(true)
}

// InFlight returns the number of currently active requests.
func (gs *GracefulShutdown) InFlight() int64 {
	return gs.inFlight.Load()
}

// IsDraining returns whether the server is in drain mode.
func (gs *GracefulShutdown) IsDraining() bool {
	return gs.draining.Load()
}
