package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Canary Deploy ───────────────────────────────────────────────────────────

func TestCanary_Get_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/canary", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp CanaryConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Enabled {
		t.Error("expected enabled=false for default canary config")
	}
	if resp.WeightNew != 0 {
		t.Errorf("expected weight_new=0, got %d", resp.WeightNew)
	}
	if resp.WeightOld != 100 {
		t.Errorf("expected weight_old=100, got %d", resp.WeightOld)
	}
}

func TestCanary_Start_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	body, _ := json.Marshal(CanaryConfig{
		NewImage:    "myapp:v2",
		WeightNew:   20,
		AutoPromote: true,
		StepPercent: 10,
		StepDelay:   30,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/canary", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["status"] != "canary_active" {
		t.Errorf("expected status=canary_active, got %v", resp["status"])
	}

	cfg, ok := resp["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config object in response")
	}
	if cfg["enabled"] != true {
		t.Error("expected config.enabled=true")
	}
	if int(cfg["weight_new"].(float64)) != 20 {
		t.Errorf("expected weight_new=20, got %v", cfg["weight_new"])
	}
	if int(cfg["weight_old"].(float64)) != 80 {
		t.Errorf("expected weight_old=80, got %v", cfg["weight_old"])
	}
}

func TestCanary_Start_DefaultWeights(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	body, _ := json.Marshal(CanaryConfig{
		NewImage:  "myapp:v2",
		WeightNew: 0, // Invalid, should default to 10
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/canary", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	cfg := resp["config"].(map[string]any)
	if int(cfg["weight_new"].(float64)) != 10 {
		t.Errorf("expected default weight_new=10, got %v", cfg["weight_new"])
	}
	if int(cfg["weight_old"].(float64)) != 90 {
		t.Errorf("expected weight_old=90, got %v", cfg["weight_old"])
	}
	if int(cfg["step_percent"].(float64)) != 10 {
		t.Errorf("expected default step_percent=10, got %v", cfg["step_percent"])
	}
	if int(cfg["step_delay"].(float64)) != 60 {
		t.Errorf("expected default step_delay=60, got %v", cfg["step_delay"])
	}
}

func TestCanary_Start_MissingImage(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	body, _ := json.Marshal(CanaryConfig{
		NewImage:  "",
		WeightNew: 20,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/canary", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "new_image required")
}

func TestCanary_Start_InvalidJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/canary", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCanary_Promote_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/canary/promote", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Promote(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %q", resp["app_id"])
	}
	if resp["status"] == "" {
		t.Error("expected non-empty status")
	}
}

func TestCanary_Cancel_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	events := testCore().Events
	handler := NewCanaryHandler(store, events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1/canary", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Cancel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %q", resp["app_id"])
	}
	if resp["status"] == "" {
		t.Error("expected non-empty status")
	}
}
