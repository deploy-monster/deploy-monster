package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── App Metrics History ─────────────────────────────────────────────────────

func TestAppMetrics_DefaultPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewMetricsHistoryHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/metrics", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.AppMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id 'app1', got %v", resp["app_id"])
	}
	if resp["period"] != "24h" {
		t.Errorf("expected default period '24h', got %v", resp["period"])
	}
}

func TestAppMetrics_1hPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewMetricsHistoryHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/metrics?period=1h", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.AppMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["period"] != "1h" {
		t.Errorf("expected period '1h', got %v", resp["period"])
	}
}

func TestAppMetrics_7dPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewMetricsHistoryHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/metrics?period=7d", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.AppMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["period"] != "7d" {
		t.Errorf("expected period '7d', got %v", resp["period"])
	}
}

func TestAppMetrics_30dPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewMetricsHistoryHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/metrics?period=30d", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.AppMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["period"] != "30d" {
		t.Errorf("expected period '30d', got %v", resp["period"])
	}
}

// ─── Server Metrics ──────────────────────────────────────────────────────────

func TestServerMetrics_Success(t *testing.T) {
	store := newMockStore()
	handler := NewMetricsHistoryHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/srv1/metrics", nil)
	req.SetPathValue("id", "srv1")
	rr := httptest.NewRecorder()

	handler.ServerMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["server_id"] != "srv1" {
		t.Errorf("expected server_id 'srv1', got %v", resp["server_id"])
	}
	if resp["period"] != "24h" {
		t.Errorf("expected default period '24h', got %v", resp["period"])
	}
}

func TestServerMetrics_CustomPeriod(t *testing.T) {
	store := newMockStore()
	handler := NewMetricsHistoryHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/srv1/metrics?period=7d", nil)
	req.SetPathValue("id", "srv1")
	rr := httptest.NewRecorder()

	handler.ServerMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["period"] != "7d" {
		t.Errorf("expected period '7d', got %v", resp["period"])
	}
}

// ─── Metrics Export ──────────────────────────────────────────────────────────

func TestMetricsExport_JSONFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app12345", TenantID: "t1", Name: "App"})
	handler := NewMetricsExportHandler(store, newMockBoltStore(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app12345/metrics/export?format=json", nil)
	req.SetPathValue("id", "app12345")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app12345" {
		t.Errorf("expected app_id 'app12345', got %v", resp["app_id"])
	}

	points, ok := resp["points"].([]any)
	if !ok {
		t.Fatal("expected points array")
	}
	if len(points) != 24 {
		t.Errorf("expected 24 points, got %d", len(points))
	}
}

func TestMetricsExport_CSVFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app12345", TenantID: "t1", Name: "App"})
	handler := NewMetricsExportHandler(store, newMockBoltStore(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app12345/metrics/export?format=csv", nil)
	req.SetPathValue("id", "app12345")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected Content-Type 'text/csv', got %q", ct)
	}

	disposition := rr.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment; filename=") {
		t.Errorf("expected Content-Disposition attachment, got %q", disposition)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "timestamp,cpu_percent,memory_mb,requests") {
		t.Error("expected CSV header row")
	}

	lines := strings.Split(strings.TrimSpace(body), "\n")
	// 1 header + 24 data rows
	if len(lines) != 25 {
		t.Errorf("expected 25 lines (header + 24 rows), got %d", len(lines))
	}
}

func TestMetricsExport_DefaultFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app12345", TenantID: "t1", Name: "App"})
	handler := NewMetricsExportHandler(store, newMockBoltStore(), nil)

	// No format param — defaults to JSON
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app12345/metrics/export", nil)
	req.SetPathValue("id", "app12345")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected default Content-Type 'application/json', got %q", ct)
	}
}
