package ingress

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// AccessLogger records ingress proxy request metrics.
type AccessLogger struct {
	logger  *slog.Logger
	metrics *AccessMetrics
}

// AccessMetrics holds aggregated ingress traffic metrics.
type AccessMetrics struct {
	TotalRequests  atomic.Int64
	TotalBytes     atomic.Int64
	StatusCounts   [6]atomic.Int64 // Index by status/100 (1xx=0, 2xx=1, 3xx=2, 4xx=3, 5xx=4, other=5)
	LatencySum     atomic.Int64    // Total latency in microseconds
}

// NewAccessLogger creates an access logger.
func NewAccessLogger(logger *slog.Logger) *AccessLogger {
	return &AccessLogger{
		logger:  logger,
		metrics: &AccessMetrics{},
	}
}

// Middleware returns HTTP middleware that logs each proxied request.
func (al *AccessLogger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusResponseWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(sw, r)

		duration := time.Since(start)
		al.metrics.TotalRequests.Add(1)
		al.metrics.LatencySum.Add(duration.Microseconds())

		// Count by status category
		idx := sw.status / 100
		if idx >= 1 && idx <= 5 {
			al.metrics.StatusCounts[idx-1].Add(1)
		} else {
			al.metrics.StatusCounts[5].Add(1)
		}

		al.logger.Debug("access",
			"method", r.Method,
			"host", r.Host,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration_ms", duration.Milliseconds(),
			"ip", clientIP(r),
			"ua", r.UserAgent(),
		)
	})
}

// Stats returns current access metrics.
func (al *AccessLogger) Stats() map[string]any {
	total := al.metrics.TotalRequests.Load()
	var avgLatency float64
	if total > 0 {
		avgLatency = float64(al.metrics.LatencySum.Load()) / float64(total) / 1000.0 // ms
	}

	return map[string]any{
		"total_requests": total,
		"status_2xx":     al.metrics.StatusCounts[1].Load(),
		"status_3xx":     al.metrics.StatusCounts[2].Load(),
		"status_4xx":     al.metrics.StatusCounts[3].Load(),
		"status_5xx":     al.metrics.StatusCounts[4].Load(),
		"avg_latency_ms": avgLatency,
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}
