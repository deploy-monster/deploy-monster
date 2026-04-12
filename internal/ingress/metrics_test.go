package ingress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsCollector_New(t *testing.T) {
	m := NewMetricsCollector()
	if m == nil {
		t.Fatal("NewMetricsCollector returned nil")
	}
}

func TestMetricsCollector_RecordRequest(t *testing.T) {
	m := NewMetricsCollector()

	// Record a successful request
	m.RecordRequest("example.com", 200, 1000, 100, 200)

	snapshot := m.Snapshot()
	if snapshot.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", snapshot.TotalRequests)
	}
	if snapshot.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", snapshot.ErrorCount)
	}
	if snapshot.BytesIn != 100 {
		t.Errorf("BytesIn = %d, want 100", snapshot.BytesIn)
	}
	if snapshot.BytesOut != 200 {
		t.Errorf("BytesOut = %d, want 200", snapshot.BytesOut)
	}
	if snapshot.LatencyAvg != 1000 {
		t.Errorf("LatencyAvg = %.2f, want 1000", snapshot.LatencyAvg)
	}
	if snapshot.RequestsByHost["example.com"] != 1 {
		t.Errorf("RequestsByHost[example.com] = %d, want 1", snapshot.RequestsByHost["example.com"])
	}
	if snapshot.RequestsByStatus["200"] != 1 {
		t.Errorf("RequestsByStatus[200] = %d, want 1", snapshot.RequestsByStatus["200"])
	}
}

func TestMetricsCollector_RecordRequest_Error(t *testing.T) {
	m := NewMetricsCollector()

	// Record an error request
	m.RecordRequest("example.com", 500, 500, 50, 100)

	snapshot := m.Snapshot()
	if snapshot.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", snapshot.TotalRequests)
	}
	if snapshot.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", snapshot.ErrorCount)
	}
}

func TestMetricsCollector_RecordRequest_Multiple(t *testing.T) {
	m := NewMetricsCollector()

	// Record multiple requests
	m.RecordRequest("example.com", 200, 1000, 100, 200)
	m.RecordRequest("example.com", 200, 2000, 150, 300)
	m.RecordRequest("api.example.com", 201, 500, 50, 100)
	m.RecordRequest("example.com", 404, 200, 20, 50)
	m.RecordRequest("example.com", 500, 300, 10, 20)

	snapshot := m.Snapshot()
	if snapshot.TotalRequests != 5 {
		t.Errorf("TotalRequests = %d, want 5", snapshot.TotalRequests)
	}
	// Only 500 should count as error
	if snapshot.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", snapshot.ErrorCount)
	}
	if snapshot.BytesIn != 330 {
		t.Errorf("BytesIn = %d, want 330", snapshot.BytesIn)
	}
	if snapshot.BytesOut != 670 {
		t.Errorf("BytesOut = %d, want 670", snapshot.BytesOut)
	}
	// Average latency: (1000 + 2000 + 500 + 200 + 300) / 5 = 800
	if snapshot.LatencyAvg != 800 {
		t.Errorf("LatencyAvg = %.2f, want 800", snapshot.LatencyAvg)
	}
	if snapshot.RequestsByHost["example.com"] != 4 {
		t.Errorf("RequestsByHost[example.com] = %d, want 4", snapshot.RequestsByHost["example.com"])
	}
	if snapshot.RequestsByHost["api.example.com"] != 1 {
		t.Errorf("RequestsByHost[api.example.com] = %d, want 1", snapshot.RequestsByHost["api.example.com"])
	}
}

func TestMetricsCollector_ActiveRequests(t *testing.T) {
	m := NewMetricsCollector()

	m.IncrementActive()
	if m.ActiveRequests() != 1 {
		t.Errorf("ActiveRequests = %d, want 1", m.ActiveRequests())
	}

	m.IncrementActive()
	if m.ActiveRequests() != 2 {
		t.Errorf("ActiveRequests = %d, want 2", m.ActiveRequests())
	}

	m.DecrementActive()
	if m.ActiveRequests() != 1 {
		t.Errorf("ActiveRequests = %d, want 1", m.ActiveRequests())
	}

	m.DecrementActive()
	if m.ActiveRequests() != 0 {
		t.Errorf("ActiveRequests = %d, want 0", m.ActiveRequests())
	}
}

func TestMetricsCollector_LatencyAvg_Zero(t *testing.T) {
	m := NewMetricsCollector()

	// No requests yet
	if m.latencyAvg() != 0 {
		t.Errorf("latencyAvg = %.2f, want 0", m.latencyAvg())
	}
}

func TestMetricsCollector_GetterMethods(t *testing.T) {
	m := NewMetricsCollector()

	m.RecordRequest("example.com", 200, 1000, 100, 200)
	m.RecordRequest("example.com", 500, 500, 50, 100)
	m.IncrementActive()

	if m.TotalRequests() != 2 {
		t.Errorf("TotalRequests() = %d, want 2", m.TotalRequests())
	}
	if m.ActiveRequests() != 1 {
		t.Errorf("ActiveRequests() = %d, want 1", m.ActiveRequests())
	}
	if m.ErrorCount() != 1 {
		t.Errorf("ErrorCount() = %d, want 1", m.ErrorCount())
	}
}

func TestPrometheusHandler_Success(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "example.com", Backends: []string{"127.0.0.1:3000"}})

	m := &Module{
		router: rt,
		proxy:  NewReverseProxy(rt, nil),
	}

	// Record some metrics
	m.proxy.metrics.RecordRequest("example.com", 200, 1000, 100, 200)
	m.proxy.metrics.RecordRequest("example.com", 500, 500, 50, 100)
	m.proxy.metrics.IncrementActive()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	m.PrometheusHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Check for expected metrics
	tests := []string{
		"ingress_requests_total 2",
		"ingress_requests_active 1",
		"ingress_errors_total 1",
		"ingress_bytes_in_total 150",
		"ingress_bytes_out_total 300",
		"ingress_latency_avg_microseconds",
		"ingress_host_requests_total{host=\"example.com\"} 2",
		"ingress_status_requests_total{status=\"200\"} 1",
		"ingress_status_requests_total{status=\"500\"} 1",
		"ingress_routes 1",
		"# TYPE ingress_requests_total counter",
		"# HELP ingress_requests_total Total number of requests processed",
	}

	for _, expected := range tests {
		if !strings.Contains(body, expected) {
			t.Errorf("expected body to contain %q", expected)
		}
	}

	// Check content type
	if rr.Header().Get("Content-Type") != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want text/plain; version=0.0.4", rr.Header().Get("Content-Type"))
	}
}

func TestPrometheusHandler_NilProxy(t *testing.T) {
	m := &Module{
		router: nil,
		proxy:  nil,
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	m.PrometheusHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}
}

func TestPrometheusHandler_NilMetrics(t *testing.T) {
	m := &Module{
		router: NewRouteTable(),
		proxy:  &ReverseProxy{metrics: nil},
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	m.PrometheusHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}
}

func TestPrometheusHandler_CircuitBreaker(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "example.com", Backends: []string{"127.0.0.1:3000"}})

	m := &Module{
		router: rt,
		proxy:  NewReverseProxy(rt, nil),
	}

	// Record some failures to create circuit breaker entry
	m.proxy.circuit.RecordFailure("127.0.0.1:3000")

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	m.PrometheusHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Check for circuit breaker metrics
	if !strings.Contains(body, "ingress_circuit_state") {
		t.Error("expected body to contain ingress_circuit_state")
	}
	if !strings.Contains(body, "backend=\"127.0.0.1:3000\"") {
		t.Error("expected body to contain backend label")
	}
}

func TestPrometheusHandler_NoRoutes(t *testing.T) {
	m := &Module{
		router: NewRouteTable(),
		proxy:  NewReverseProxy(NewRouteTable(), nil),
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	m.PrometheusHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Should have 0 routes
	if !strings.Contains(body, "ingress_routes 0") {
		t.Error("expected ingress_routes 0")
	}
}

func TestSyncMapInc_NilMap(t *testing.T) {
	var m syncMapCount

	m.Inc("key1")
	m.Inc("key1")
	m.Inc("key2")

	counts := m.GetAll()
	if counts["key1"] != 2 {
		t.Errorf("counts[key1] = %d, want 2", counts["key1"])
	}
	if counts["key2"] != 1 {
		t.Errorf("counts[key2] = %d, want 1", counts["key2"])
	}
}

func TestSyncMapInc_GetAll_Empty(t *testing.T) {
	m := syncMapCount{counts: make(map[string]int64)}

	counts := m.GetAll()
	if len(counts) != 0 {
		t.Errorf("expected empty map, got %d entries", len(counts))
	}
}
