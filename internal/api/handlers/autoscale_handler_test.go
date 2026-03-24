package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Get Autoscale ───────────────────────────────────────────────────────────

func TestAutoscale_Get_Success(t *testing.T) {
	store := newMockStore()
	handler := NewAutoscaleHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/autoscale", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var cfg AutoscaleConfig
	json.Unmarshal(rr.Body.Bytes(), &cfg)

	if cfg.Enabled != false {
		t.Errorf("expected enabled=false, got %v", cfg.Enabled)
	}
	if cfg.MinReplicas != 1 {
		t.Errorf("expected min_replicas=1, got %d", cfg.MinReplicas)
	}
	if cfg.MaxReplicas != 10 {
		t.Errorf("expected max_replicas=10, got %d", cfg.MaxReplicas)
	}
	if cfg.CPUTarget != 80 {
		t.Errorf("expected cpu_target=80, got %d", cfg.CPUTarget)
	}
	if cfg.RAMTarget != 85 {
		t.Errorf("expected ram_target=85, got %d", cfg.RAMTarget)
	}
	if cfg.ScaleUpDelay != 60 {
		t.Errorf("expected scale_up_delay=60, got %d", cfg.ScaleUpDelay)
	}
	if cfg.ScaleDownDelay != 300 {
		t.Errorf("expected scale_down_delay=300, got %d", cfg.ScaleDownDelay)
	}
}

// ─── Update Autoscale ────────────────────────────────────────────────────────

func TestAutoscale_Update_Success(t *testing.T) {
	store := newMockStore()
	handler := NewAutoscaleHandler(store)

	body, _ := json.Marshal(AutoscaleConfig{
		Enabled:     true,
		MinReplicas: 2,
		MaxReplicas: 20,
		CPUTarget:   70,
		RAMTarget:   75,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/autoscale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}

	cfg, ok := resp["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config object in response")
	}
	if cfg["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", cfg["enabled"])
	}
	if int(cfg["min_replicas"].(float64)) != 2 {
		t.Errorf("expected min_replicas=2, got %v", cfg["min_replicas"])
	}
	if int(cfg["max_replicas"].(float64)) != 20 {
		t.Errorf("expected max_replicas=20, got %v", cfg["max_replicas"])
	}
}

func TestAutoscale_Update_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewAutoscaleHandler(store)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/autoscale", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestAutoscale_Update_MaxLessThanMin(t *testing.T) {
	store := newMockStore()
	handler := NewAutoscaleHandler(store)

	body, _ := json.Marshal(AutoscaleConfig{
		Enabled:     true,
		MinReplicas: 5,
		MaxReplicas: 2, // less than min — should be clamped to min
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/autoscale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	cfg := resp["config"].(map[string]any)
	if int(cfg["max_replicas"].(float64)) != 5 {
		t.Errorf("expected max_replicas clamped to min=5, got %v", cfg["max_replicas"])
	}
}

func TestAutoscale_Update_NegativeMin(t *testing.T) {
	store := newMockStore()
	handler := NewAutoscaleHandler(store)

	body, _ := json.Marshal(AutoscaleConfig{
		MinReplicas: -1,
		MaxReplicas: 10,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/autoscale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	cfg := resp["config"].(map[string]any)
	if int(cfg["min_replicas"].(float64)) != 1 {
		t.Errorf("expected min_replicas clamped to 1, got %v", cfg["min_replicas"])
	}
}
