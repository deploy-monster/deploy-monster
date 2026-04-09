package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// APIMetrics collects HTTP request metrics for the management API.
type APIMetrics struct {
	totalRequests  atomic.Int64
	activeRequests atomic.Int64
	totalErrors    atomic.Int64
	totalLatencyUS atomic.Int64 // microseconds cumulative
	totalBytesOut  atomic.Int64 // total response bytes
	statusCounts   sync.Map     // status code string -> *atomic.Int64
	endpointCounts sync.Map     // "METHOD /path" -> *atomic.Int64

	// Business metrics (incremented via event subscriptions)
	deploysTotal  atomic.Int64
	deploysFailed atomic.Int64
	buildsTotal   atomic.Int64
	buildsFailed  atomic.Int64
	appsCreated   atomic.Int64
	appsDeleted   atomic.Int64
	eventBus      *core.EventBus // optional, for Stats() in handler
}

// NewAPIMetrics creates a new API metrics collector.
func NewAPIMetrics() *APIMetrics {
	return &APIMetrics{}
}

// SubscribeEvents subscribes to the event bus to track business-level metrics.
func (m *APIMetrics) SubscribeEvents(eb *core.EventBus) {
	m.eventBus = eb
	eb.SubscribeAsync(core.EventDeployFinished, func(_ context.Context, _ core.Event) error {
		m.deploysTotal.Add(1)
		return nil
	})
	eb.SubscribeAsync(core.EventDeployFailed, func(_ context.Context, _ core.Event) error {
		m.deploysTotal.Add(1)
		m.deploysFailed.Add(1)
		return nil
	})
	eb.SubscribeAsync(core.EventBuildCompleted, func(_ context.Context, _ core.Event) error {
		m.buildsTotal.Add(1)
		return nil
	})
	eb.SubscribeAsync(core.EventBuildFailed, func(_ context.Context, _ core.Event) error {
		m.buildsTotal.Add(1)
		m.buildsFailed.Add(1)
		return nil
	})
	eb.SubscribeAsync(core.EventAppCreated, func(_ context.Context, _ core.Event) error {
		m.appsCreated.Add(1)
		return nil
	})
	eb.SubscribeAsync(core.EventAppDeleted, func(_ context.Context, _ core.Event) error {
		m.appsDeleted.Add(1)
		return nil
	})
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
		m.totalBytesOut.Add(int64(sw.bytesWritten))

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

		sb.WriteString("\n# HELP api_response_bytes_total Total response bytes sent\n")
		sb.WriteString("# TYPE api_response_bytes_total counter\n")
		sb.WriteString(fmt.Sprintf("api_response_bytes_total %d\n", m.totalBytesOut.Load()))

		sb.WriteString("\n# HELP api_requests_by_status Total requests by status code\n")
		sb.WriteString("# TYPE api_requests_by_status counter\n")
		m.statusCounts.Range(func(key, value any) bool {
			k, _ := key.(string)
			v, _ := value.(*atomic.Int64)
			if k != "" && v != nil {
				sb.WriteString(fmt.Sprintf("api_requests_by_status{status=%q} %d\n", k, v.Load()))
			}
			return true
		})

		sb.WriteString("\n# HELP api_requests_by_endpoint Total requests by endpoint\n")
		sb.WriteString("# TYPE api_requests_by_endpoint counter\n")
		m.endpointCounts.Range(func(key, value any) bool {
			k, _ := key.(string)
			v, _ := value.(*atomic.Int64)
			if k != "" && v != nil {
				sb.WriteString(fmt.Sprintf("api_requests_by_endpoint{endpoint=%q} %d\n", k, v.Load()))
			}
			return true
		})

		// Business metrics
		sb.WriteString("\n# HELP deploys_total Total deployments (success + failure)\n")
		sb.WriteString("# TYPE deploys_total counter\n")
		sb.WriteString(fmt.Sprintf("deploys_total %d\n", m.deploysTotal.Load()))

		sb.WriteString("\n# HELP deploys_failed_total Failed deployments\n")
		sb.WriteString("# TYPE deploys_failed_total counter\n")
		sb.WriteString(fmt.Sprintf("deploys_failed_total %d\n", m.deploysFailed.Load()))

		sb.WriteString("\n# HELP builds_total Total builds (success + failure)\n")
		sb.WriteString("# TYPE builds_total counter\n")
		sb.WriteString(fmt.Sprintf("builds_total %d\n", m.buildsTotal.Load()))

		sb.WriteString("\n# HELP builds_failed_total Failed builds\n")
		sb.WriteString("# TYPE builds_failed_total counter\n")
		sb.WriteString(fmt.Sprintf("builds_failed_total %d\n", m.buildsFailed.Load()))

		sb.WriteString("\n# HELP apps_created_total Total apps created\n")
		sb.WriteString("# TYPE apps_created_total counter\n")
		sb.WriteString(fmt.Sprintf("apps_created_total %d\n", m.appsCreated.Load()))

		sb.WriteString("\n# HELP apps_deleted_total Total apps deleted\n")
		sb.WriteString("# TYPE apps_deleted_total counter\n")
		sb.WriteString(fmt.Sprintf("apps_deleted_total %d\n", m.appsDeleted.Load()))

		// Event bus stats
		if m.eventBus != nil {
			stats := m.eventBus.Stats()
			sb.WriteString("\n# HELP eventbus_published_total Total events published\n")
			sb.WriteString("# TYPE eventbus_published_total counter\n")
			sb.WriteString(fmt.Sprintf("eventbus_published_total %d\n", stats.PublishCount))

			sb.WriteString("\n# HELP eventbus_errors_total Total event handler errors\n")
			sb.WriteString("# TYPE eventbus_errors_total counter\n")
			sb.WriteString(fmt.Sprintf("eventbus_errors_total %d\n", stats.ErrorCount))

			sb.WriteString("\n# HELP eventbus_subscriptions Active event subscriptions\n")
			sb.WriteString("# TYPE eventbus_subscriptions gauge\n")
			sb.WriteString(fmt.Sprintf("eventbus_subscriptions %d\n", stats.SubscriptionCount))

			sb.WriteString("\n# HELP eventbus_async_pool_size Max concurrent async handlers\n")
			sb.WriteString("# TYPE eventbus_async_pool_size gauge\n")
			sb.WriteString(fmt.Sprintf("eventbus_async_pool_size %d\n", stats.AsyncPoolSize))

			sb.WriteString("\n# HELP eventbus_async_pool_active Currently active async handlers\n")
			sb.WriteString("# TYPE eventbus_async_pool_active gauge\n")
			sb.WriteString(fmt.Sprintf("eventbus_async_pool_active %d\n", stats.AsyncPoolActive))
		}

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
