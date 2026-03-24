package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── HealthCheck Get ─────────────────────────────────────────────────────────

func TestHealthCheckGet_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:     "app1",
		Name:   "Test App",
		Status: "running",
	})

	handler := NewHealthCheckHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/healthcheck", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var cfg HealthCheckConfig
	json.Unmarshal(rr.Body.Bytes(), &cfg)

	if cfg.Type != "http" {
		t.Errorf("expected type=http, got %q", cfg.Type)
	}
	if cfg.Path != "/health" {
		t.Errorf("expected path=/health, got %q", cfg.Path)
	}
	if cfg.Interval != 10 {
		t.Errorf("expected interval=10, got %d", cfg.Interval)
	}
	if cfg.Timeout != 5 {
		t.Errorf("expected timeout=5, got %d", cfg.Timeout)
	}
	if cfg.Retries != 3 {
		t.Errorf("expected retries=3, got %d", cfg.Retries)
	}
}

func TestHealthCheckGet_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewHealthCheckHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent/healthcheck", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

// ─── HealthCheck Update ──────────────────────────────────────────────────────

func TestHealthCheckUpdate_Success(t *testing.T) {
	store := newMockStore()
	handler := NewHealthCheckHandler(store)

	body, _ := json.Marshal(HealthCheckConfig{
		Type:     "tcp",
		Port:     5432,
		Interval: 30,
		Timeout:  10,
		Retries:  5,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/healthcheck", bytes.NewReader(body))
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

	config := resp["config"].(map[string]any)
	if config["type"] != "tcp" {
		t.Errorf("expected config type=tcp, got %v", config["type"])
	}
	if int(config["interval"].(float64)) != 30 {
		t.Errorf("expected interval=30, got %v", config["interval"])
	}
}

func TestHealthCheckUpdate_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewHealthCheckHandler(store)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/healthcheck", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestHealthCheckUpdate_InvalidType(t *testing.T) {
	store := newMockStore()
	handler := NewHealthCheckHandler(store)

	body, _ := json.Marshal(HealthCheckConfig{
		Type:     "grpc",
		Interval: 10,
		Timeout:  5,
		Retries:  3,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/healthcheck", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "type must be: http, tcp, exec, none")
}

func TestHealthCheckUpdate_DefaultValues(t *testing.T) {
	store := newMockStore()
	handler := NewHealthCheckHandler(store)

	// Provide type but no interval/timeout/retries — should get defaults.
	body, _ := json.Marshal(HealthCheckConfig{
		Type: "exec",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/healthcheck", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	config := resp["config"].(map[string]any)
	if int(config["interval"].(float64)) != 10 {
		t.Errorf("expected default interval=10, got %v", config["interval"])
	}
	if int(config["timeout"].(float64)) != 5 {
		t.Errorf("expected default timeout=5, got %v", config["timeout"])
	}
	if int(config["retries"].(float64)) != 3 {
		t.Errorf("expected default retries=3, got %v", config["retries"])
	}
}

func TestHealthCheckUpdate_NoneType(t *testing.T) {
	store := newMockStore()
	handler := NewHealthCheckHandler(store)

	body, _ := json.Marshal(HealthCheckConfig{
		Type:     "none",
		Interval: 10,
		Timeout:  5,
		Retries:  3,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/healthcheck", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ─── Detailed Health ─────────────────────────────────────────────────────────

func TestDetailedHealth_Success(t *testing.T) {
	c := testCore()
	c.Store = newMockStore()
	c.Registry = core.NewRegistry()
	c.Build = core.BuildInfo{Version: "1.0.0-test"}

	handler := NewDetailedHealthHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/health/detailed", nil)
	rr := httptest.NewRecorder()

	handler.DetailedHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "healthy" {
		t.Errorf("expected status=healthy, got %v", resp["status"])
	}
	if resp["version"] != "1.0.0-test" {
		t.Errorf("expected version=1.0.0-test, got %v", resp["version"])
	}

	checks, ok := resp["checks"].(map[string]any)
	if !ok {
		t.Fatal("expected checks map in response")
	}

	// Database check
	db, ok := checks["database"].(map[string]any)
	if !ok {
		t.Fatal("expected database check")
	}
	if db["healthy"] != true {
		t.Errorf("expected database healthy=true, got %v", db["healthy"])
	}

	// Docker check (should be false — no runtime configured)
	docker, ok := checks["docker"].(map[string]any)
	if !ok {
		t.Fatal("expected docker check")
	}
	if docker["healthy"] != false {
		t.Errorf("expected docker healthy=false (nil runtime), got %v", docker["healthy"])
	}

	// Events check
	events, ok := checks["events"].(map[string]any)
	if !ok {
		t.Fatal("expected events check")
	}
	if events["healthy"] != true {
		t.Errorf("expected events healthy=true, got %v", events["healthy"])
	}

	// Runtime check
	rt, ok := checks["runtime"].(map[string]any)
	if !ok {
		t.Fatal("expected runtime check")
	}
	if rt["healthy"] != true {
		t.Errorf("expected runtime healthy=true, got %v", rt["healthy"])
	}

	if resp["duration"] == nil || resp["duration"] == "" {
		t.Error("expected non-empty duration")
	}
}

func TestDetailedHealth_NilStore(t *testing.T) {
	c := testCore()
	c.Store = nil // no store
	c.Registry = core.NewRegistry()

	handler := NewDetailedHealthHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/health/detailed", nil)
	rr := httptest.NewRecorder()

	handler.DetailedHealth(rr, req)

	// Should return 503 because database check fails.
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "degraded" {
		t.Errorf("expected status=degraded, got %v", resp["status"])
	}
}

func TestDetailedHealth_WithDockerRuntime(t *testing.T) {
	c := testCore()
	c.Store = newMockStore()
	c.Registry = core.NewRegistry()
	c.Services.Container = &mockContainerRuntime{}

	handler := NewDetailedHealthHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/health/detailed", nil)
	rr := httptest.NewRecorder()

	handler.DetailedHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	checks := resp["checks"].(map[string]any)
	docker := checks["docker"].(map[string]any)
	if docker["healthy"] != true {
		t.Errorf("expected docker healthy=true with runtime, got %v", docker["healthy"])
	}
}
