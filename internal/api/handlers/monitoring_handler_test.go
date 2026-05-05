package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// monitoringTestCore returns a minimal core.Core for MonitoringHandler.
// Services.Container is left nil so the container-listing branch falls
// through to the all-zeros path; that keeps the test free of a runtime
// dependency while still exercising every line MonitoringHandler.Metrics
// can take in that mode.
func monitoringTestCore() *core.Core {
	return &core.Core{
		Config:   &core.Config{},
		Build:    core.BuildInfo{Version: "test"},
		Registry: core.NewRegistry(),
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
}

func TestMonitoringHandler_New(t *testing.T) {
	startedAt := time.Now()
	h := NewMonitoringHandler(monitoringTestCore(), startedAt)
	if h == nil {
		t.Fatal("NewMonitoringHandler returned nil")
	}
	if !h.startedAt.Equal(startedAt) {
		t.Fatalf("startedAt not stored: got %v, want %v", h.startedAt, startedAt)
	}
	if h.reader == nil {
		t.Fatal("reader was not initialised")
	}
}

func TestMonitoringHandler_Metrics(t *testing.T) {
	startedAt := time.Now().Add(-30 * time.Second)
	h := NewMonitoringHandler(monitoringTestCore(), startedAt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/server", nil)
	rr := httptest.NewRecorder()

	h.Metrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}

	for _, key := range []string{
		"cpu_percent", "memory_used", "memory_total", "disk_used",
		"disk_total", "network_rx", "network_tx", "uptime",
		"containers_running", "containers_total", "load_avg",
	} {
		if _, ok := resp[key]; !ok {
			t.Errorf("response missing key %q: %v", key, resp)
		}
	}

	// With Services.Container == nil the container counts must be zero —
	// this is the branch the test deliberately exercises.
	if got, _ := resp["containers_running"].(float64); got != 0 {
		t.Errorf("containers_running = %v, want 0 when no container runtime", got)
	}
	if got, _ := resp["containers_total"].(float64); got != 0 {
		t.Errorf("containers_total = %v, want 0 when no container runtime", got)
	}

	// Uptime must reflect the time since startedAt — at least 30s, but
	// allow generous slack for slow CI machines.
	uptime, ok := resp["uptime"].(float64)
	if !ok {
		t.Fatalf("uptime is not a number: %T", resp["uptime"])
	}
	if uptime < 30 {
		t.Errorf("uptime = %v, want >= 30", uptime)
	}

	loadAvg, ok := resp["load_avg"].([]any)
	if !ok || len(loadAvg) != 3 {
		t.Errorf("load_avg should be a 3-element array, got %v", resp["load_avg"])
	}
}

func TestMonitoringHandler_Alerts(t *testing.T) {
	h := NewMonitoringHandler(monitoringTestCore(), time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rr := httptest.NewRecorder()
	h.Alerts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Metric    string `json:"metric"`
			Threshold int    `json:"threshold"`
			Status    string `json:"status"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}

	if resp.Total != 3 || len(resp.Data) != 3 {
		t.Fatalf("expected 3 alerts, got total=%d data=%d", resp.Total, len(resp.Data))
	}

	wantIDs := map[string]string{
		"cpu-high":    "cpu",
		"memory-high": "memory",
		"disk-high":   "disk",
	}
	for _, a := range resp.Data {
		wantMetric, ok := wantIDs[a.ID]
		if !ok {
			t.Errorf("unexpected alert id %q", a.ID)
			continue
		}
		if a.Metric != wantMetric {
			t.Errorf("alert %s metric=%q, want %q", a.ID, a.Metric, wantMetric)
		}
		if a.Threshold != 90 {
			t.Errorf("alert %s threshold=%d, want 90", a.ID, a.Threshold)
		}
		if a.Status != "ok" {
			t.Errorf("alert %s status=%q, want ok", a.ID, a.Status)
		}
	}
}
