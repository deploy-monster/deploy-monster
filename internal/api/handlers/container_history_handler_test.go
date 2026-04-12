package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Container History ───────────────────────────────────────────────────────

func TestContainerHistory_DefaultPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewContainerHistoryHandler(store, runtime, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/containers/history", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["period"] != "1h" {
		t.Errorf("expected default period=1h, got %v", resp["period"])
	}
	if int(resp["count"].(float64)) != 60 {
		t.Errorf("expected count=60 for 1h period, got %v", resp["count"])
	}

	points, ok := resp["points"].([]any)
	if !ok {
		t.Fatal("expected points array in response")
	}
	if len(points) != 60 {
		t.Errorf("expected 60 points, got %d", len(points))
	}
}

func TestContainerHistory_24hPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewContainerHistoryHandler(store, runtime, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/containers/history?period=24h", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["period"] != "24h" {
		t.Errorf("expected period=24h, got %v", resp["period"])
	}
	if int(resp["count"].(float64)) != 96 {
		t.Errorf("expected count=96 for 24h period, got %v", resp["count"])
	}

	points := resp["points"].([]any)
	if len(points) != 96 {
		t.Errorf("expected 96 points, got %d", len(points))
	}
}

func TestContainerHistory_7dPeriod(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewContainerHistoryHandler(store, runtime, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/containers/history?period=7d", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["period"] != "7d" {
		t.Errorf("expected period=7d, got %v", resp["period"])
	}
	if int(resp["count"].(float64)) != 168 {
		t.Errorf("expected count=168 for 7d period, got %v", resp["count"])
	}

	points := resp["points"].([]any)
	if len(points) != 168 {
		t.Errorf("expected 168 points, got %d", len(points))
	}
}

func TestContainerHistory_UnknownPeriodDefaultsTo1h(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewContainerHistoryHandler(store, runtime, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/containers/history?period=30d", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	// Unknown period falls through to default case with count=60.
	if int(resp["count"].(float64)) != 60 {
		t.Errorf("expected count=60 for unknown period, got %v", resp["count"])
	}
}

func TestContainerHistory_PointStructure(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewContainerHistoryHandler(store, runtime, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/containers/history", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	points := resp["points"].([]any)
	if len(points) == 0 {
		t.Fatal("expected non-empty points array")
	}

	point := points[0].(map[string]any)
	requiredFields := []string{"timestamp", "cpu_percent", "memory_mb", "memory_max_mb", "net_rx_kb", "net_tx_kb", "pids"}
	for _, field := range requiredFields {
		if _, ok := point[field]; !ok {
			t.Errorf("expected field %q in point, not found", field)
		}
	}
}
