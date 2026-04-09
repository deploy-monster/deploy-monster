package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── App Middleware ──────────────────────────────────────────────────────────

func TestAppMiddleware_Get_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewAppMiddlewareHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/middleware", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp MiddlewareConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.Compress {
		t.Error("expected compress=true by default")
	}
}

func TestAppMiddleware_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewAppMiddlewareHandler(store, newMockBoltStore())

	body, _ := json.Marshal(MiddlewareConfig{
		RateLimit: &RateLimitMiddleware{
			Enabled:        true,
			RequestsPerMin: 100,
			BurstSize:      20,
			By:             "ip",
		},
		CORS: &CORSMiddleware{
			Enabled:        true,
			AllowedOrigins: []string{"https://example.com"},
			AllowedMethods: []string{"GET", "POST"},
			AllowedHeaders: []string{"Authorization", "Content-Type"},
			MaxAge:         3600,
		},
		Compress: true,
		Headers: map[string]string{
			"X-Custom-Header": "value",
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/middleware", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
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
	if cfg["compress"] != true {
		t.Error("expected config.compress=true")
	}

	rateLimitCfg, ok := cfg["rate_limit"].(map[string]any)
	if !ok {
		t.Fatal("expected rate_limit object in config")
	}
	if rateLimitCfg["enabled"] != true {
		t.Error("expected rate_limit.enabled=true")
	}
	if int(rateLimitCfg["requests_per_min"].(float64)) != 100 {
		t.Errorf("expected requests_per_min=100, got %v", rateLimitCfg["requests_per_min"])
	}

	corsCfg, ok := cfg["cors"].(map[string]any)
	if !ok {
		t.Fatal("expected cors object in config")
	}
	if corsCfg["enabled"] != true {
		t.Error("expected cors.enabled=true")
	}
	origins := corsCfg["allowed_origins"].([]any)
	if len(origins) != 1 || origins[0] != "https://example.com" {
		t.Errorf("expected allowed_origins=[https://example.com], got %v", origins)
	}
}

func TestAppMiddleware_Update_MinimalConfig(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewAppMiddlewareHandler(store, newMockBoltStore())

	body, _ := json.Marshal(MiddlewareConfig{
		Compress: false,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/middleware", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}
}

func TestAppMiddleware_Update_InvalidJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewAppMiddlewareHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/middleware", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}
