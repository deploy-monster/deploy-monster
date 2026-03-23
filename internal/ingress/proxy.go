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
)

// ReverseProxy is the core HTTP handler that routes incoming requests
// to backend containers based on the route table.
type ReverseProxy struct {
	router  *RouteTable
	logger  *slog.Logger
	metrics *ProxyMetrics
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
	return &ReverseProxy{
		router:  router,
		logger:  logger,
		metrics: &ProxyMetrics{},
	}
}

// ServeHTTP implements http.Handler — the main request processing pipeline.
// Flow: Match route → Apply middleware → Load balance → Reverse proxy
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.metrics.TotalRequests.Add(1)
	rp.metrics.ActiveRequests.Add(1)
	defer rp.metrics.ActiveRequests.Add(-1)

	start := time.Now()

	// 1. Find matching route
	host := extractHost(r.Host)
	route := rp.router.Match(host, r.URL.Path)

	if route == nil {
		rp.logger.Debug("no route matched", "host", host, "path", r.URL.Path)
		http.Error(w, "502 Bad Gateway — no upstream configured", http.StatusBadGateway)
		rp.metrics.ErrorCount.Add(1)
		return
	}

	// 2. Pick backend using load balancer strategy
	if len(route.Backends) == 0 {
		http.Error(w, "503 Service Unavailable — no healthy backends", http.StatusServiceUnavailable)
		rp.metrics.ErrorCount.Add(1)
		return
	}

	backend := pickBackend(route)

	// 3. Build reverse proxy for this backend
	targetURL, err := url.Parse("http://" + backend)
	if err != nil {
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		rp.metrics.ErrorCount.Add(1)
		return
	}

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
			rp.metrics.ErrorCount.Add(1)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)

	rp.logger.Debug("proxy request",
		"host", host,
		"path", r.URL.Path,
		"backend", backend,
		"duration", time.Since(start).String(),
	)
}

// Metrics returns the current proxy metrics.
func (rp *ReverseProxy) Metrics() ProxyMetrics {
	return ProxyMetrics{
		TotalRequests:  atomic.Int64{},
		ActiveRequests: atomic.Int64{},
		ErrorCount:     atomic.Int64{},
	}
}

// pickBackend selects a backend from the route's backend pool.
// For now uses simple round-robin; will be replaced by LB module.
var rrCounter atomic.Uint64

func pickBackend(route *RouteEntry) string {
	idx := rrCounter.Add(1) - 1
	return route.Backends[idx%uint64(len(route.Backends))]
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

// ErrorPage returns a styled error page.
func ErrorPage(status int, title, message string) []byte {
	return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%d %s</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f8fafc;color:#0f172a}
.box{text-align:center;padding:2rem}.code{font-size:4rem;font-weight:700;color:#10b981}p{color:#64748b;margin-top:.5rem}</style>
</head><body><div class="box"><div class="code">%d</div><h1>%s</h1><p>%s</p><p style="margin-top:2rem;font-size:.75rem;color:#94a3b8">DeployMonster Ingress</p></div></body></html>`,
		status, title, status, title, message))
}
