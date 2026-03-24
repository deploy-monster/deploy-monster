package ingress

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAccessLogger(t *testing.T) {
	logger := slog.Default()
	al := NewAccessLogger(logger)

	if al == nil {
		t.Fatal("expected non-nil AccessLogger")
	}
	if al.logger != logger {
		t.Error("expected logger to be set")
	}
	if al.metrics == nil {
		t.Error("expected metrics to be initialized")
	}
}

func TestAccessLogger_Middleware_StatusCapture(t *testing.T) {
	al := NewAccessLogger(slog.Default())

	tests := []struct {
		name       string
		status     int
		statusIdx  int // index into StatusCounts (0-based: 1xx=0, 2xx=1, ...)
	}{
		{"200 OK", http.StatusOK, 1},
		{"201 Created", http.StatusCreated, 1},
		{"301 Redirect", http.StatusMovedPermanently, 2},
		{"404 Not Found", http.StatusNotFound, 3},
		{"500 Server Error", http.StatusInternalServerError, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			al = NewAccessLogger(slog.Default())

			handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "10.0.0.1:1234"
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if al.metrics.TotalRequests.Load() != 1 {
				t.Errorf("expected TotalRequests=1, got %d", al.metrics.TotalRequests.Load())
			}

			count := al.metrics.StatusCounts[tt.statusIdx].Load()
			if count != 1 {
				t.Errorf("expected StatusCounts[%d]=1, got %d", tt.statusIdx, count)
			}
		})
	}
}

func TestAccessLogger_Middleware_MultipleRequests(t *testing.T) {
	al := NewAccessLogger(slog.Default())

	handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))

	for range 5 {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	if al.metrics.TotalRequests.Load() != 5 {
		t.Errorf("expected TotalRequests=5, got %d", al.metrics.TotalRequests.Load())
	}
	if al.metrics.StatusCounts[1].Load() != 5 { // 2xx index
		t.Errorf("expected 5 2xx responses, got %d", al.metrics.StatusCounts[1].Load())
	}
}

func TestAccessLogger_Middleware_DefaultStatus(t *testing.T) {
	al := NewAccessLogger(slog.Default())

	// Handler that doesn't explicitly call WriteHeader (defaults to 200)
	handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("implicit 200"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Default status should be 200 (2xx -> index 1)
	if al.metrics.StatusCounts[1].Load() != 1 {
		t.Errorf("expected 1 2xx response for implicit 200, got %d", al.metrics.StatusCounts[1].Load())
	}
}

func TestAccessLogger_Stats(t *testing.T) {
	al := NewAccessLogger(slog.Default())

	// Verify empty stats
	stats := al.Stats()
	if stats["total_requests"].(int64) != 0 {
		t.Errorf("expected total_requests=0, got %v", stats["total_requests"])
	}
	if stats["avg_latency_ms"].(float64) != 0 {
		t.Errorf("expected avg_latency_ms=0, got %v", stats["avg_latency_ms"])
	}

	// Make some requests with different status codes
	makeReq := func(status int) {
		handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	makeReq(200)
	makeReq(200)
	makeReq(301)
	makeReq(404)
	makeReq(500)

	stats = al.Stats()
	if stats["total_requests"].(int64) != 5 {
		t.Errorf("expected total_requests=5, got %v", stats["total_requests"])
	}
	if stats["status_2xx"].(int64) != 2 {
		t.Errorf("expected status_2xx=2, got %v", stats["status_2xx"])
	}
	if stats["status_3xx"].(int64) != 1 {
		t.Errorf("expected status_3xx=1, got %v", stats["status_3xx"])
	}
	if stats["status_4xx"].(int64) != 1 {
		t.Errorf("expected status_4xx=1, got %v", stats["status_4xx"])
	}
	if stats["status_5xx"].(int64) != 1 {
		t.Errorf("expected status_5xx=1, got %v", stats["status_5xx"])
	}

	// avg_latency_ms should be > 0 now (we had requests)
	avgLatency := stats["avg_latency_ms"].(float64)
	if avgLatency < 0 {
		t.Errorf("expected avg_latency_ms >= 0, got %v", avgLatency)
	}
}

func TestStatusResponseWriter_WriteHeader(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusResponseWriter{ResponseWriter: inner, status: 200}

	sw.WriteHeader(http.StatusNotFound)

	if sw.status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", sw.status)
	}
	if inner.Code != http.StatusNotFound {
		t.Errorf("expected inner recorder status 404, got %d", inner.Code)
	}
}

func TestStatusResponseWriter_Write(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusResponseWriter{ResponseWriter: inner, status: 200}

	data := []byte("test body content")
	n, err := sw.Write(data)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}
	if sw.bytes != int64(len(data)) {
		t.Errorf("expected bytes=%d, got %d", len(data), sw.bytes)
	}

	// Write again
	n2, err := sw.Write([]byte("more"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw.bytes != int64(len(data)+n2) {
		t.Errorf("expected accumulated bytes=%d, got %d", len(data)+n2, sw.bytes)
	}
}

func TestAccessLogger_Middleware_LatencyTracked(t *testing.T) {
	al := NewAccessLogger(slog.Default())

	handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if al.metrics.LatencySum.Load() < 0 {
		t.Error("expected non-negative latency sum")
	}
}
