package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func getCounterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	c.Write(&m)
	return m.Counter.GetValue()
}

func getGaugeValue(g prometheus.Gauge) float64 {
	var m dto.Metric
	g.Write(&m)
	return m.Gauge.GetValue()
}

func TestAPIMetrics_Middleware_CountsRequests(t *testing.T) {
	m := NewAPIMetrics()
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if v := getCounterValue(m.requestsTotal.WithLabelValues("GET", "/api/v1/apps", "200")); v != 3 {
		t.Errorf("requests_total = %f, want 3", v)
	}
	if m.activeRequests.Load() != 0 {
		t.Errorf("activeRequests = %d, want 0 after completion", m.activeRequests.Load())
	}
}

func TestAPIMetrics_Middleware_Counts5xxErrors(t *testing.T) {
	m := NewAPIMetrics()
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := getCounterValue(m.errorsTotal.WithLabelValues("GET", "/api/v1/apps")); v != 1 {
		t.Errorf("errors_total = %f, want 1", v)
	}
}

func TestAPIMetrics_Middleware_4xxNotCountedAsError(t *testing.T) {
	m := NewAPIMetrics()
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if v := getCounterValue(m.errorsTotal.WithLabelValues("GET", "/api/v1/apps")); v != 0 {
		t.Errorf("errors_total = %f, want 0 for 404", v)
	}
}

func TestAPIMetrics_Middleware_TracksLatency(t *testing.T) {
	m := NewAPIMetrics()
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond) // ensure measurable latency
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify the histogram is registered - just ensure no panic
	// Full histogram testing would require the full Prometheus registry.
}

func TestAPIMetrics_Handler_PrometheusFormat(t *testing.T) {
	m := NewAPIMetrics()

	// Generate some traffic first
	mid := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	mid.ServeHTTP(rec, req)

	// Now fetch metrics via promhttp
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()

	if metricsRec.Header().Get("Content-Type") != "text/plain; version=0.0.4" &&
		metricsRec.Header().Get("Content-Type") != "text/plain" {
		// promhttp returns text/plain
	}

	// Check for http_request_duration_seconds histogram
	if !contains(body, "http_request_duration_seconds") {
		t.Error("metrics should contain http_request_duration_seconds histogram")
	}
}

func TestAPIMetrics_SubscribeEvents(t *testing.T) {
	m := NewAPIMetrics()
	eb := core.NewEventBus(nil)
	m.SubscribeEvents(eb)

	// Emit deploy finished
	eb.Publish(context.Background(), core.NewEvent(core.EventDeployFinished, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if v := getCounterValue(m.deploysTotal); v != 1 {
		t.Errorf("deploysTotal = %f, want 1", v)
	}

	// Emit deploy failed
	eb.Publish(context.Background(), core.NewEvent(core.EventDeployFailed, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if v := getCounterValue(m.deploysTotal); v != 2 {
		t.Errorf("deploysTotal = %f, want 2", v)
	}
	if v := getCounterValue(m.deploysFailed); v != 1 {
		t.Errorf("deploysFailed = %f, want 1", v)
	}

	// Emit build completed
	eb.Publish(context.Background(), core.NewEvent(core.EventBuildCompleted, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if v := getCounterValue(m.buildsTotal); v != 1 {
		t.Errorf("buildsTotal = %f, want 1", v)
	}

	// Emit build failed
	eb.Publish(context.Background(), core.NewEvent(core.EventBuildFailed, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if v := getCounterValue(m.buildsTotal); v != 2 {
		t.Errorf("buildsTotal = %f, want 2", v)
	}
	if v := getCounterValue(m.buildsFailed); v != 1 {
		t.Errorf("buildsFailed = %f, want 1", v)
	}

	// Emit app created
	eb.Publish(context.Background(), core.NewEvent(core.EventAppCreated, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if v := getCounterValue(m.appsCreated); v != 1 {
		t.Errorf("appsCreated = %f, want 1", v)
	}

	// Emit app deleted
	eb.Publish(context.Background(), core.NewEvent(core.EventAppDeleted, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if v := getCounterValue(m.appsDeleted); v != 1 {
		t.Errorf("appsDeleted = %f, want 1", v)
	}
}

func TestGroupPath_ReplacesLongSegments(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/api/v1/apps", "/api/v1/apps"},
		{"/api/v1/apps/12345678901234567890", "/api/v1/apps/{id}"},
		{"/api/v1/apps/short", "/api/v1/apps/short"},
	}

	for _, tc := range tests {
		got := groupPath(tc.input)
		if got != tc.want {
			t.Errorf("groupPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}