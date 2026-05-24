package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// APIMetrics collects HTTP request metrics for the management API.
type APIMetrics struct {
	registry *prometheus.Registry
	// Prometheus histogram for request latency (P50/P90/P95/P99).
	latencyHistogram *prometheus.HistogramVec
	// Prometheus counter for total requests.
	requestsTotal *prometheus.CounterVec
	// Prometheus counter for total response bytes.
	bytesOutTotal prometheus.Counter
	// Prometheus gauge for active requests.
	activeRequests atomic.Int64
	// Prometheus counter for 5xx errors.
	errorsTotal *prometheus.CounterVec

	// Business metrics (incremented via event subscriptions)
	deploysTotal  prometheus.Counter
	deploysFailed prometheus.Counter
	buildsTotal   prometheus.Counter
	buildsFailed  prometheus.Counter
	appsCreated   prometheus.Counter
	appsDeleted   prometheus.Counter
	eventBus      *core.EventBus // optional, for Stats() in handler
}

// NewAPIMetrics creates a new API metrics collector with Prometheus histogram.
func NewAPIMetrics() *APIMetrics {
	reg := prometheus.NewRegistry()
	m := &APIMetrics{
		registry: reg,
		latencyHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency in seconds.",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "endpoint", "status_code"},
		),
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests.",
			},
			[]string{"method", "endpoint", "status_code"},
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_errors_total",
				Help: "Total number of HTTP 5xx errors.",
			},
			[]string{"method", "endpoint"},
		),
		bytesOutTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http_response_bytes_total",
			Help: "Total response bytes sent.",
		}),
		deploysTotal:  prometheus.NewCounter(prometheus.CounterOpts{Name: "deploymonster_deploys_total", Help: "Total deployments."}),
		deploysFailed: prometheus.NewCounter(prometheus.CounterOpts{Name: "deploymonster_deploys_failed_total", Help: "Failed deployments."}),
		buildsTotal:   prometheus.NewCounter(prometheus.CounterOpts{Name: "deploymonster_builds_total", Help: "Total builds."}),
		buildsFailed:  prometheus.NewCounter(prometheus.CounterOpts{Name: "deploymonster_builds_failed_total", Help: "Failed builds."}),
		appsCreated:   prometheus.NewCounter(prometheus.CounterOpts{Name: "deploymonster_apps_created_total", Help: "Apps created."}),
		appsDeleted:   prometheus.NewCounter(prometheus.CounterOpts{Name: "deploymonster_apps_deleted_total", Help: "Apps deleted."}),
	}
	reg.MustRegister(m.latencyHistogram, m.requestsTotal, m.errorsTotal, m.bytesOutTotal,
		m.deploysTotal, m.deploysFailed, m.buildsTotal, m.buildsFailed, m.appsCreated, m.appsDeleted)
	return m
}

// SubscribeEvents subscribes to the event bus to track business-level metrics.
func (m *APIMetrics) SubscribeEvents(eb *core.EventBus) {
	m.eventBus = eb
	eb.SubscribeAsync(core.EventDeployFinished, func(_ context.Context, _ core.Event) error {
		m.deploysTotal.Inc()
		return nil
	})
	eb.SubscribeAsync(core.EventDeployFailed, func(_ context.Context, _ core.Event) error {
		m.deploysTotal.Inc()
		m.deploysFailed.Inc()
		return nil
	})
	eb.SubscribeAsync(core.EventBuildCompleted, func(_ context.Context, _ core.Event) error {
		m.buildsTotal.Inc()
		return nil
	})
	eb.SubscribeAsync(core.EventBuildFailed, func(_ context.Context, _ core.Event) error {
		m.buildsTotal.Inc()
		m.buildsFailed.Inc()
		return nil
	})
	eb.SubscribeAsync(core.EventAppCreated, func(_ context.Context, _ core.Event) error {
		m.appsCreated.Inc()
		return nil
	})
	eb.SubscribeAsync(core.EventAppDeleted, func(_ context.Context, _ core.Event) error {
		m.appsDeleted.Inc()
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
		duration := time.Since(start).Seconds()
		statusCode := fmt.Sprintf("%d", sw.status)
		endpoint := groupPath(r.URL.Path)

		m.latencyHistogram.WithLabelValues(r.Method, endpoint, statusCode).Observe(duration)
		m.requestsTotal.WithLabelValues(r.Method, endpoint, statusCode).Inc()
		m.bytesOutTotal.Add(float64(sw.bytesWritten))

		if sw.status >= 500 {
			m.errorsTotal.WithLabelValues(r.Method, endpoint).Inc()
		}
	})
}

// Handler returns an HTTP handler that exposes metrics via the Prometheus registry.
func (m *APIMetrics) Handler() http.HandlerFunc {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true}).ServeHTTP
}

// TotalBytesOut returns the total response bytes as an atomic int64.
// This exists for test compatibility - the actual counter is a Prometheus counter.
func (m *APIMetrics) TotalBytesOut() int64 {
	var m2 dto.Metric
	if err := m.bytesOutTotal.Write(&m2); err != nil {
		return 0
	}
	return int64(m2.Counter.GetValue())
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
