package ingress

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// MetricsCollector collects and exposes ingress metrics.
type MetricsCollector struct {
	totalRequests    atomic.Int64
	activeRequests   atomic.Int64
	errorCount       atomic.Int64
	bytesIn          atomic.Int64
	bytesOut         atomic.Int64
	latencySum       atomic.Int64 // in microseconds
	latencyCount     atomic.Int64
	requestsByHost   syncMapCount // host -> count
	requestsByStatus syncMapCount // status code -> count
}

// syncMapCount is a thread-safe map for counting.
type syncMapCount struct {
	mu     sync.RWMutex
	counts map[string]int64
}

func (m *syncMapCount) Inc(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts == nil {
		m.counts = make(map[string]int64)
	}
	m.counts[key]++
}

func (m *syncMapCount) GetAll() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]int64, len(m.counts))
	for k, v := range m.counts {
		result[k] = v
	}
	return result
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestsByHost:   syncMapCount{counts: make(map[string]int64)},
		requestsByStatus: syncMapCount{counts: make(map[string]int64)},
	}
}

// RecordRequest records a completed request.
func (m *MetricsCollector) RecordRequest(host string, statusCode int, latencyMicros int64, bytesIn, bytesOut int64) {
	m.totalRequests.Add(1)
	m.latencySum.Add(latencyMicros)
	m.latencyCount.Add(1)
	m.bytesIn.Add(bytesIn)
	m.bytesOut.Add(bytesOut)

	m.requestsByHost.Inc(host)
	m.requestsByStatus.Inc(fmt.Sprintf("%d", statusCode))

	// Only 5xx are server errors (4xx are client errors)
	if statusCode >= 500 {
		m.errorCount.Add(1)
	}
}

// IncrementActive increments the active request counter.
func (m *MetricsCollector) IncrementActive() {
	m.activeRequests.Add(1)
}

// DecrementActive decrements the active request counter.
func (m *MetricsCollector) DecrementActive() {
	m.activeRequests.Add(-1)
}

// Snapshot returns a snapshot of current metrics.
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalRequests:    m.totalRequests.Load(),
		ActiveRequests:   m.activeRequests.Load(),
		ErrorCount:       m.errorCount.Load(),
		BytesIn:          m.bytesIn.Load(),
		BytesOut:         m.bytesOut.Load(),
		LatencyAvg:       m.latencyAvg(),
		RequestsByHost:   m.requestsByHost.GetAll(),
		RequestsByStatus: m.requestsByStatus.GetAll(),
	}
}

func (m *MetricsCollector) latencyAvg() float64 {
	count := m.latencyCount.Load()
	if count == 0 {
		return 0
	}
	return float64(m.latencySum.Load()) / float64(count)
}

// MetricsSnapshot is a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	TotalRequests    int64
	ActiveRequests   int64
	ErrorCount       int64
	BytesIn          int64
	BytesOut         int64
	LatencyAvg       float64 // in microseconds
	RequestsByHost   map[string]int64
	RequestsByStatus map[string]int64
}

// TotalRequests returns the total number of requests.
func (m *MetricsCollector) TotalRequests() int64 {
	return m.totalRequests.Load()
}

// ActiveRequests returns the number of active requests.
func (m *MetricsCollector) ActiveRequests() int64 {
	return m.activeRequests.Load()
}

// ErrorCount returns the total error count.
func (m *MetricsCollector) ErrorCount() int64 {
	return m.errorCount.Load()
}

// PrometheusHandler returns a handler that exposes metrics in Prometheus format.
func (m *Module) PrometheusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.proxy == nil || m.proxy.metrics == nil {
			http.Error(w, "metrics not available", http.StatusServiceUnavailable)
			return
		}

		// Get metrics from proxy
		snapshot := m.proxy.metrics.Snapshot()

		var sb strings.Builder

		// Help and type declarations
		sb.WriteString("# HELP ingress_requests_total Total number of requests processed\n")
		sb.WriteString("# TYPE ingress_requests_total counter\n")
		sb.WriteString(fmt.Sprintf("ingress_requests_total %d\n", snapshot.TotalRequests))

		sb.WriteString("\n# HELP ingress_requests_active Number of active requests\n")
		sb.WriteString("# TYPE ingress_requests_active gauge\n")
		sb.WriteString(fmt.Sprintf("ingress_requests_active %d\n", snapshot.ActiveRequests))

		sb.WriteString("\n# HELP ingress_errors_total Total number of errors\n")
		sb.WriteString("# TYPE ingress_errors_total counter\n")
		sb.WriteString(fmt.Sprintf("ingress_errors_total %d\n", snapshot.ErrorCount))

		sb.WriteString("\n# HELP ingress_bytes_in_total Total bytes received\n")
		sb.WriteString("# TYPE ingress_bytes_in_total counter\n")
		sb.WriteString(fmt.Sprintf("ingress_bytes_in_total %d\n", snapshot.BytesIn))

		sb.WriteString("\n# HELP ingress_bytes_out_total Total bytes sent\n")
		sb.WriteString("# TYPE ingress_bytes_out_total counter\n")
		sb.WriteString(fmt.Sprintf("ingress_bytes_out_total %d\n", snapshot.BytesOut))

		sb.WriteString("\n# HELP ingress_latency_avg_microseconds Average request latency\n")
		sb.WriteString("# TYPE ingress_latency_avg_microseconds gauge\n")
		sb.WriteString(fmt.Sprintf("ingress_latency_avg_microseconds %.2f\n", snapshot.LatencyAvg))

		// Requests by host
		sb.WriteString("\n# HELP ingress_host_requests_total Requests per host\n")
		sb.WriteString("# TYPE ingress_host_requests_total counter\n")
		for host, count := range snapshot.RequestsByHost {
			sb.WriteString(fmt.Sprintf("ingress_host_requests_total{host=%q} %d\n", host, count))
		}

		// Requests by status
		sb.WriteString("\n# HELP ingress_status_requests_total Requests per status code\n")
		sb.WriteString("# TYPE ingress_status_requests_total counter\n")
		for status, count := range snapshot.RequestsByStatus {
			sb.WriteString(fmt.Sprintf("ingress_status_requests_total{status=%q} %d\n", status, count))
		}

		// Route count
		routeCount := 0
		if m.router != nil {
			m.router.mu.RLock()
			routeCount = len(m.router.routes)
			m.router.mu.RUnlock()
		}
		sb.WriteString("\n# HELP ingress_routes Number of configured routes\n")
		sb.WriteString("# TYPE ingress_routes gauge\n")
		sb.WriteString(fmt.Sprintf("ingress_routes %d\n", routeCount))

		// Circuit breaker stats
		if m.proxy.circuit != nil {
			sb.WriteString("\n# HELP ingress_circuit_state Circuit breaker state (0=closed, 1=open, 2=half-open)\n")
			sb.WriteString("# TYPE ingress_circuit_state gauge\n")
			for backend, stats := range m.proxy.circuit.AllStats() {
				sb.WriteString(fmt.Sprintf("ingress_circuit_state{backend=%q} %d\n", backend, stats.State))
			}
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(sb.String()))
	}
}
