package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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

	if m.totalRequests.Load() != 3 {
		t.Errorf("totalRequests = %d, want 3", m.totalRequests.Load())
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

	if m.totalErrors.Load() != 1 {
		t.Errorf("totalErrors = %d, want 1", m.totalErrors.Load())
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

	if m.totalErrors.Load() != 0 {
		t.Errorf("totalErrors = %d, want 0 for 404", m.totalErrors.Load())
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

	if m.totalLatencyUS.Load() <= 0 {
		t.Errorf("latency should be positive after a request, got %d", m.totalLatencyUS.Load())
	}
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

	// Now fetch metrics
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()

	if metricsRec.Header().Get("Content-Type") != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want Prometheus text format", metricsRec.Header().Get("Content-Type"))
	}

	expectedMetrics := []string{
		"api_requests_total 1",
		"api_requests_active 0",
		"api_errors_total 0",
		"api_latency_avg_microseconds",
		"deploys_total 0",
		"builds_total 0",
		"apps_created_total 0",
	}
	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("metrics output missing %q", metric)
		}
	}
}

func TestAPIMetrics_Handler_StatusCounts(t *testing.T) {
	m := NewAPIMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	errHandler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/bad", nil)
	rec2 := httptest.NewRecorder()
	errHandler.ServeHTTP(rec2, req2)

	// Check metrics output contains both status codes
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, `status="200"`) {
		t.Error("metrics should contain status 200 count")
	}
	if !strings.Contains(body, `status="400"`) {
		t.Error("metrics should contain status 400 count")
	}
}

func TestAPIMetrics_SubscribeEvents(t *testing.T) {
	m := NewAPIMetrics()
	eb := core.NewEventBus(nil)
	m.SubscribeEvents(eb)

	// Emit deploy finished
	eb.Publish(context.Background(), core.NewEvent(core.EventDeployFinished, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if m.deploysTotal.Load() != 1 {
		t.Errorf("deploysTotal = %d, want 1", m.deploysTotal.Load())
	}

	// Emit deploy failed
	eb.Publish(context.Background(), core.NewEvent(core.EventDeployFailed, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if m.deploysTotal.Load() != 2 {
		t.Errorf("deploysTotal = %d, want 2", m.deploysTotal.Load())
	}
	if m.deploysFailed.Load() != 1 {
		t.Errorf("deploysFailed = %d, want 1", m.deploysFailed.Load())
	}

	// Emit build completed
	eb.Publish(context.Background(), core.NewEvent(core.EventBuildCompleted, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if m.buildsTotal.Load() != 1 {
		t.Errorf("buildsTotal = %d, want 1", m.buildsTotal.Load())
	}

	// Emit build failed
	eb.Publish(context.Background(), core.NewEvent(core.EventBuildFailed, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if m.buildsTotal.Load() != 2 {
		t.Errorf("buildsTotal = %d, want 2", m.buildsTotal.Load())
	}
	if m.buildsFailed.Load() != 1 {
		t.Errorf("buildsFailed = %d, want 1", m.buildsFailed.Load())
	}

	// Emit app created
	eb.Publish(context.Background(), core.NewEvent(core.EventAppCreated, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if m.appsCreated.Load() != 1 {
		t.Errorf("appsCreated = %d, want 1", m.appsCreated.Load())
	}

	// Emit app deleted
	eb.Publish(context.Background(), core.NewEvent(core.EventAppDeleted, "test", nil))
	time.Sleep(50 * time.Millisecond)
	if m.appsDeleted.Load() != 1 {
		t.Errorf("appsDeleted = %d, want 1", m.appsDeleted.Load())
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
