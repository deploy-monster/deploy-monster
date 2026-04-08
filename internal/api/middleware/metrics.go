package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// APIMetrics collects HTTP request metrics for the management API.
type APIMetrics struct {
	totalRequests  atomic.Int64
	activeRequests atomic.Int64
	totalErrors    atomic.Int64
	totalLatencyUS atomic.Int64 // microseconds cumulative
	statusCounts   sync.Map     // status code string -> *atomic.Int64
	endpointCounts sync.Map     // "METHOD /path" -> *atomic.Int64
}

// NewAPIMetrics creates a new API metrics collector.
func NewAPIMetrics() *APIMetrics {
	return &APIMetrics{}
}

// Middleware returns an HTTP middleware that records request metrics.
func (m *APIMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.activeRequests.Add(1)
		start := time.Now()

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		m.activeRequests.Add(-1)
		m.totalRequests.Add(1)
		m.totalLatencyUS.Add(time.Since(start).Microseconds())

		if sw.status >= 500 {
			m.totalErrors.Add(1)
		}

		// Count by status code
		statusKey := fmt.Sprintf("%d", sw.status)
		counter := m.getOrCreateCounter(&m.statusCounts, statusKey)
		counter.Add(1)

		// Count by endpoint (method + first path segment for grouping)
		endpoint := r.Method + " " + groupPath(r.URL.Path)
		epCounter := m.getOrCreateCounter(&m.endpointCounts, endpoint)
		epCounter.Add(1)
	})
}

// Handler returns an HTTP handler that exposes metrics in Prometheus text format.
func (m *APIMetrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder

		total := m.totalRequests.Load()
		active := m.activeRequests.Load()
		errors := m.totalErrors.Load()
		latencyUS := m.totalLatencyUS.Load()

		var avgLatency float64
		if total > 0 {
			avgLatency = float64(latencyUS) / float64(total)
		}

		sb.WriteString("# HELP api_requests_total Total API requests\n")
		sb.WriteString("# TYPE api_requests_total counter\n")
		sb.WriteString(fmt.Sprintf("api_requests_total %d\n", total))

		sb.WriteString("\n# HELP api_requests_active Currently active API requests\n")
		sb.WriteString("# TYPE api_requests_active gauge\n")
		sb.WriteString(fmt.Sprintf("api_requests_active %d\n", active))

		sb.WriteString("\n# HELP api_errors_total Total 5xx errors\n")
		sb.WriteString("# TYPE api_errors_total counter\n")
		sb.WriteString(fmt.Sprintf("api_errors_total %d\n", errors))

		sb.WriteString("\n# HELP api_latency_avg_microseconds Average request latency\n")
		sb.WriteString("# TYPE api_latency_avg_microseconds gauge\n")
		sb.WriteString(fmt.Sprintf("api_latency_avg_microseconds %.2f\n", avgLatency))

		sb.WriteString("\n# HELP api_requests_by_status Total requests by status code\n")
		sb.WriteString("# TYPE api_requests_by_status counter\n")
		m.statusCounts.Range(func(key, value any) bool {
			sb.WriteString(fmt.Sprintf("api_requests_by_status{status=%q} %d\n", key.(string), value.(*atomic.Int64).Load()))
			return true
		})

		sb.WriteString("\n# HELP api_requests_by_endpoint Total requests by endpoint\n")
		sb.WriteString("# TYPE api_requests_by_endpoint counter\n")
		m.endpointCounts.Range(func(key, value any) bool {
			sb.WriteString(fmt.Sprintf("api_requests_by_endpoint{endpoint=%q} %d\n", key.(string), value.(*atomic.Int64).Load()))
			return true
		})

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(sb.String()))
	}
}

func (m *APIMetrics) getOrCreateCounter(sm *sync.Map, key string) *atomic.Int64 {
	if v, ok := sm.Load(key); ok {
		return v.(*atomic.Int64)
	}
	counter := &atomic.Int64{}
	actual, _ := sm.LoadOrStore(key, counter)
	return actual.(*atomic.Int64)
}

// groupPath normalizes URL paths for metric grouping — replaces path IDs with {id}.
func groupPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		// Heuristic: if a segment looks like an ID (long hex, UUID, or has dashes+digits), replace it
		if len(p) >= 8 && !strings.ContainsAny(p, "abcdefghijklmnopqrstuvwxyz") {
			parts[i] = "{id}"
		} else if len(p) >= 20 {
			parts[i] = "{id}"
		}
	}
	return "/" + strings.Join(parts, "/")
}
