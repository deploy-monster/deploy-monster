package ingress

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/deploy/graceful"
	"github.com/deploy-monster/deploy-monster/internal/ingress/lb"
)

// ReverseProxy is the core HTTP handler that routes incoming requests
// to backend containers based on the route table.
type ReverseProxy struct {
	router  *RouteTable
	logger  *slog.Logger
	metrics *MetricsCollector
	tracker *graceful.ConnectionTracker     // Track active connections
	drainer *graceful.DrainManager          // Manage draining backends
	circuit *graceful.CircuitBreakerManager // Circuit breaker for failing backends
	lb      lb.Strategy
}

// ProxyMetrics tracks ingress proxy statistics.
type ProxyMetrics struct {
	TotalRequests  atomic.Int64
	ActiveRequests atomic.Int64
	ErrorCount     atomic.Int64
	BytesIn        atomic.Int64
	BytesOut       atomic.Int64
}

// NewReverseProxy creates a new reverse proxy handler.
func NewReverseProxy(router *RouteTable, logger *slog.Logger) *ReverseProxy {
	tracker := graceful.NewConnectionTracker()
	return &ReverseProxy{
		router:  router,
		logger:  logger,
		metrics: NewMetricsCollector(),
		tracker: tracker,
		drainer: graceful.NewDrainManager(tracker),
		circuit: graceful.NewCircuitBreakerManager(graceful.DefaultCircuitConfig()),
		lb:      lb.New("round-robin"),
	}
}

// DrainBackend marks a backend as draining and waits for connections to complete.
// During draining, the backend is excluded from load balancing while existing
// connections are allowed to complete.
func (rp *ReverseProxy) DrainBackend(backend string, timeout time.Duration) error {
	return rp.drainer.WaitForDrain(backend, timeout)
}

// StartDrain marks a backend as draining (no new connections will be routed).
// Returns active connection count and draining status.
func (rp *ReverseProxy) StartDrain(backend string) (int64, bool) {
	done := rp.drainer.StartDrain(backend)
	if done == nil {
		return 0, false // Already draining
	}
	return rp.tracker.Active(backend), true
}

// CompleteDrain signals that draining is complete for a backend.
func (rp *ReverseProxy) CompleteDrain(backend string) {
	rp.drainer.CompleteDrain(backend)
}

// IsDraining returns true if a backend is currently being drained.
func (rp *ReverseProxy) IsDraining(backend string) bool {
	return rp.drainer.IsDraining(backend)
}

// CircuitStats returns circuit breaker stats for a backend.
func (rp *ReverseProxy) CircuitStats(backend string) (graceful.CircuitStats, bool) {
	return rp.circuit.Stats(backend)
}

// AllCircuitStats returns circuit breaker stats for all backends.
func (rp *ReverseProxy) AllCircuitStats() map[string]graceful.CircuitStats {
	return rp.circuit.AllStats()
}

// ResetCircuit resets the circuit breaker for a backend.
func (rp *ReverseProxy) ResetCircuit(backend string) {
	rp.circuit.Reset(backend)
}

// ErrorPage generates an HTML error page for display.
func ErrorPage(status int, title, message string) []byte {
	return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%d %s</title>
    <style>
        body { font-family: system-ui, sans-serif; text-align: center; padding: 50px; }
        h1 { color: #333; }
        p { color: #666; }
        .footer { margin-top: 30px; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <h1>%d %s</h1>
    <p>%s</p>
    <div class="footer">DeployMonster Ingress</div>
</body>
</html>`, status, title, status, title, message))
}

// pickBackend selects a backend using round-robin from the route entry.
// This is a helper function for tests and simple use cases.
var pickBackendCounter atomic.Uint64

func pickBackend(route *RouteEntry) string {
	if len(route.Backends) == 0 {
		return ""
	}
	idx := pickBackendCounter.Add(1) - 1
	return route.Backends[idx%uint64(len(route.Backends))]
}

// ServeHTTP implements http.Handler — the main request processing pipeline.
// Flow: Match route → Filter backends → Load balance → Reverse proxy
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.metrics.IncrementActive()
	defer rp.metrics.DecrementActive()

	start := time.Now()

	// 1. Find matching route
	host := extractHost(r.Host)
	route := rp.router.Match(host, r.URL.Path)

	if route == nil {
		rp.logger.Debug("no route matched", "host", host, "path", r.URL.Path)
		http.Error(w, "502 Bad Gateway — no upstream configured", http.StatusBadGateway)
		rp.metrics.RecordRequest(host, 502, time.Since(start).Microseconds(), 0, 0)
		return
	}

	// 2. Filter out draining backends and backends with open circuits
	healthyBackends := rp.filterHealthyBackends(route.Backends)
	if len(healthyBackends) == 0 {
		http.Error(w, "503 Service Unavailable — no healthy backends", http.StatusServiceUnavailable)
		rp.metrics.RecordRequest(host, 503, time.Since(start).Microseconds(), 0, 0)
		return
	}

	// 3. Pick backend using load balancer strategy
	backend := rp.lb.Next(healthyBackends, r)

	// 4. Track connection
	rp.tracker.Increment(backend)
	defer rp.tracker.Decrement(backend)

	// 5. Build reverse proxy for this backend
	targetURL, err := url.Parse("http://" + backend)
	if err != nil {
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		rp.circuit.RecordFailure(backend)
		return
	}

	// Wrap response writer to track success/failure for circuit breaker
	rw := &responseTracker{ResponseWriter: w, backend: backend, circuit: rp.circuit}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host

			// Strip prefix if configured
			if route.StripPrefix && route.PathPrefix != "/" {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, route.PathPrefix)
				if req.URL.Path == "" {
					req.URL.Path = "/"
				}
			}

			// Forward headers
			req.Header.Set("X-Forwarded-For", clientIP(r))
			req.Header.Set("X-Forwarded-Host", r.Host)
			req.Header.Set("X-Forwarded-Proto", scheme(r))
			req.Header.Set("X-Real-IP", clientIP(r))
			req.Host = r.Host
		},
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			rp.logger.Error("proxy error",
				"host", host,
				"backend", backend,
				"error", err,
			)
			// Record failure for circuit breaker
			rp.circuit.RecordFailure(backend)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(rw, r)

	rp.logger.Debug("proxy request",
		"host", host,
		"path", r.URL.Path,
		"backend", backend,
		"status", rw.status,
		"duration", time.Since(start).String(),
	)
}

// responseTracker wraps http.ResponseWriter to track response status for circuit breaker.
type responseTracker struct {
	http.ResponseWriter
	backend string
	circuit *graceful.CircuitBreakerManager
	status  int
	written bool
}

func (rt *responseTracker) WriteHeader(status int) {
	if !rt.written {
		rt.status = status
		rt.written = true
		// Record success/failure based on status code
		if status >= 500 {
			rt.circuit.RecordFailure(rt.backend)
		} else {
			rt.circuit.RecordSuccess(rt.backend)
		}
	}
	rt.ResponseWriter.WriteHeader(status)
}

func (rt *responseTracker) Write(b []byte) (int, error) {
	if !rt.written {
		// WriteHeader wasn't called, status is 200
		rt.status = 200
		rt.circuit.RecordSuccess(rt.backend)
		rt.written = true
	}
	return rt.ResponseWriter.Write(b)
}

// Metrics returns the current proxy metrics.
func (rp *ReverseProxy) Metrics() ProxyMetrics {
	return ProxyMetrics{
		TotalRequests:  atomic.Int64{},
		ActiveRequests: atomic.Int64{},
		ErrorCount:     atomic.Int64{},
	}
}

// filterHealthyBackends removes backends that are draining or have open circuits.
func (rp *ReverseProxy) filterHealthyBackends(backends []string) []string {
	healthy := make([]string, 0, len(backends))
	for _, backend := range backends {
		// Skip draining backends
		if rp.drainer.IsDraining(backend) {
			continue
		}
		// Skip backends with open circuits (failing fast)
		if rp.circuit.IsOpen(backend) {
			continue
		}
		healthy = append(healthy, backend)
	}
	return healthy
}

// extractHost strips the port from the Host header.
func extractHost(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return host // No port in host
	}
	return h
}

// clientIP extracts the real client IP.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// scheme returns "https" or "http" based on the request.
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}
