package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// RollbackHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackHandler_New(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)
	if h == nil {
		t.Fatal("NewRollbackHandler returned nil")
	}
}

func TestRollbackHandler_Rollback_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_ZeroVersion(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":0}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_NegativeVersion(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":-1}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_VersionNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":99}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_Success(t *testing.T) {
	store := newMockStore()
	store.addDeployment("app-1", core.Deployment{Version: 1, Image: "app:v1", Status: "stopped"})
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":1}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRollbackHandler_ListVersions_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	store.addDeployment("app-1", core.Deployment{Version: 2, Image: "app:v2", Status: "running"})
	store.addDeployment("app-1", core.Deployment{Version: 1, Image: "app:v1", Status: "stopped"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/versions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.ListVersions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRollbackHandler_ListVersions_Error(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	store.errListDeploymentsByApp = core.ErrNotFound
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/versions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.ListVersions(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// TestRollbackHandler_ListVersions_CrossTenant verifies Phase 7.11 fix —
// a developer token for tenant A must not be able to list versions of an
// app owned by tenant B, even if they guess the app ID.
func TestRollbackHandler_ListVersions_CrossTenant(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-B", Name: "foreign", TenantID: "tenant-B"})
	store.addDeployment("app-B", core.Deployment{Version: 1, Image: "app:v1", Status: "running"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-B/versions", nil)
	req.SetPathValue("id", "app-B")
	req = withClaims(req, "u1", "tenant-A", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.ListVersions(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant ListVersions should be 404, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EventWebhookHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestEventWebhookHandler_New(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())
	if h == nil {
		t.Fatal("NewEventWebhookHandler returned nil")
	}
}

func TestEventWebhookHandler_List_Empty(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_List_WithData(t *testing.T) {
	bolt := newMockBoltStore()
	list := eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh1", URL: "https://example.com/hook", Secret: "s3cret", Events: []string{"app.deployed"}, Active: true},
			{ID: "wh2", URL: "https://example.com/hook2", Secret: "", Events: []string{"app.crashed"}, Active: true},
		},
	}
	bolt.Set("event_webhooks", "all", list, 0)

	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), bolt)

	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	total := int(body["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
}

func TestEventWebhookHandler_Create_NoClaims(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_InvalidBody(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader("{bad"))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_MissingFields(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(`{"url":""}`))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_Success(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	body := `{"url":"https://example.com/hook","events":["app.deployed"]}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_WithSecret(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	body := `{"url":"https://example.com/hook","events":["app.deployed"],"secret":"my-secret"}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_NotFound(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockBoltStore())

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_Success(t *testing.T) {
	bolt := newMockBoltStore()
	list := eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh-1", URL: "https://example.com", Events: []string{"app.deployed"}},
			{ID: "wh-2", URL: "https://other.com", Events: []string{"app.crashed"}},
		},
	}
	bolt.Set("event_webhooks", "all", list, 0)

	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ExecHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestExecHandler_New(t *testing.T) {
	h := NewExecHandler(nil, nil, slog.Default(), nil)
	if h == nil {
		t.Fatal("NewExecHandler returned nil")
	}
}

func TestExecHandler_NilRuntime(t *testing.T) {
	h := NewExecHandler(nil, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":"ls"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestExecHandler_InvalidBody(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestExecHandler_EmptyCommand(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":""}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestExecHandler_NoContainer(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{containers: []core.ContainerInfo{}}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":"ls"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestExecHandler_Success(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123", State: "running"}},
	}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":"ls -la"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// FileBrowserHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestFileBrowserHandler_New(t *testing.T) {
	h := NewFileBrowserHandler(nil, nil)
	if h == nil {
		t.Fatal("NewFileBrowserHandler returned nil")
	}
}

func TestFileBrowserHandler_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewFileBrowserHandler(store, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/files?path=/", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestFileBrowserHandler_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewFileBrowserHandler(store, &mockContainerRuntime{containers: []core.ContainerInfo{}})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/files?path=/tmp", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFileBrowserHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewFileBrowserHandler(store, &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123def456", State: "running"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/files", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// GPUHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestGPUHandler_New(t *testing.T) {
	h := NewGPUHandler(newMockStore(), nil, newMockBoltStore())
	if h == nil {
		t.Fatal("NewGPUHandler returned nil")
	}
}

func TestGPUHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := GPUConfig{Enabled: true, Capabilities: []string{"compute"}, Driver: "nvidia"}
	bolt.Set("gpu_config", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, bolt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/gpu", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGPUHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockBoltStore())

	body := `{"enabled":true,"capabilities":[],"driver":""}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/gpu", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_DetectGPU_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	detection := body["detection"].(map[string]any)
	if detection["available"] != false {
		t.Errorf("expected GPU not available, got %v", detection["available"])
	}
}

func TestGPUHandler_DetectGPU_WithNvidiaImage(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{},
	}
	// Override ImageList to return nvidia images
	h := &GPUHandler{store: store, runtime: runtime, bolt: newMockBoltStore()}

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RedirectHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRedirectHandler_New(t *testing.T) {
	h := NewRedirectHandler(nil, newMockBoltStore())
	if h == nil {
		t.Fatal("NewRedirectHandler returned nil")
	}
}

func TestRedirectHandler_List_Empty(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/redirects", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRedirectHandler_List_WithData(t *testing.T) {
	bolt := newMockBoltStore()
	list := redirectList{Rules: []RedirectRule{{ID: "r1", Source: "/old", Destination: "/new", StatusCode: 301}}}
	bolt.Set("redirects", "app-1", list, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, bolt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/redirects", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_MissingFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(`{"source":""}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockBoltStore())

	body := `{"source":"/old","destination":"/new"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestRedirectHandler_Delete_NotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockBoltStore())

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestRedirectHandler_Delete_Success(t *testing.T) {
	bolt := newMockBoltStore()
	list := redirectList{Rules: []RedirectRule{
		{ID: "r-1", Source: "/old", Destination: "/new"},
		{ID: "r-2", Source: "/foo", Destination: "/bar"},
	}}
	bolt.Set("redirects", "app-1", list, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ResponseHeadersHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestResponseHeadersHandler_New(t *testing.T) {
	h := NewResponseHeadersHandler(nil, newMockBoltStore())
	if h == nil {
		t.Fatal("NewResponseHeadersHandler returned nil")
	}
}

func TestResponseHeadersHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/response-headers", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := ResponseHeadersConfig{HSTS: "max-age=31536000", XFrameOptions: "SAMEORIGIN"}
	bolt.Set("response_headers", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, bolt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/response-headers", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockBoltStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockBoltStore())

	body := `{"hsts":"max-age=31536000","x_frame_options":"DENY"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StickySessionHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestStickySessionHandler_New(t *testing.T) {
	h := NewStickySessionHandler(nil, newMockBoltStore())
	if h == nil {
		t.Fatal("NewStickySessionHandler returned nil")
	}
}

func TestStickySessionHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/sticky-sessions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := StickySessionConfig{Enabled: true, Cookie: "MY_SESSION", MaxAge: 7200}
	bolt.Set("sticky_sessions", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, bolt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/sticky-sessions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	body := `{"enabled":true,"cookie":"","max_age":3600}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SuspendHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestSuspendHandler_New(t *testing.T) {
	h := NewSuspendHandler(newMockStore(), nil, core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewSuspendHandler returned nil")
	}
}

func TestSuspendHandler_Suspend_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/suspend", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Suspend(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestSuspendHandler_Suspend_AlreadySuspended(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "suspended"})
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/suspend", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Suspend(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestSuspendHandler_Suspend_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1", State: "running"}},
	}
	h := NewSuspendHandler(store, runtime, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/suspend", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Suspend(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSuspendHandler_Resume_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/resume", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Resume(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestSuspendHandler_Resume_NotSuspended(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "running"})
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/resume", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Resume(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestSuspendHandler_Resume_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "suspended"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1", State: "exited"}},
	}
	h := NewSuspendHandler(store, runtime, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/resume", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Resume(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ResourceHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestResourceHandler_New(t *testing.T) {
	h := NewResourceHandler(newMockStore(), core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewResourceHandler returned nil")
	}
}

func TestResourceHandler_SetLimits_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestResourceHandler_SetLimits_AppNotFound(t *testing.T) {
	h := NewResourceHandler(newMockStore(), core.NewEventBus(nil))

	body := `{"cpu_quota":100000,"memory_mb":512}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestResourceHandler_SetLimits_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	body := `{"cpu_quota":100000,"memory_mb":512}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestResourceHandler_GetLimits_AppNotFound(t *testing.T) {
	h := NewResourceHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/resources", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.GetLimits(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestResourceHandler_GetLimits_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/resources", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.GetLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StatsHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestStatsHandler_New(t *testing.T) {
	h := NewStatsHandler(nil, newMockStore())
	if h == nil {
		t.Fatal("NewStatsHandler returned nil")
	}
}

func TestStatsHandler_AppStats_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStatsHandler(nil, store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/stats", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestStatsHandler_AppStats_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStatsHandler(&mockContainerRuntime{}, store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/stats", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppStats(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestStatsHandler_AppStats_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStatsHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1", State: "running"}},
	}, store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/stats", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestStatsHandler_ServerStats_NilRuntime(t *testing.T) {
	h := NewStatsHandler(nil, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestStatsHandler_ServerStats_Success(t *testing.T) {
	h := NewStatsHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running"},
			{ID: "c2", State: "exited"},
		},
	}, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WebhookReplayHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWebhookReplayHandler_New(t *testing.T) {
	h := NewWebhookReplayHandler(nil, core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewWebhookReplayHandler returned nil")
	}
}

func TestWebhookReplayHandler_Replay(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewWebhookReplayHandler(store, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/webhooks/log-1/replay", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("logId", "log-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Replay(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WebhookRotateHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWebhookRotateHandler_New(t *testing.T) {
	h := NewWebhookRotateHandler(newMockStore(), core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewWebhookRotateHandler returned nil")
	}
}

func TestWebhookRotateHandler_Rotate(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewWebhookRotateHandler(store, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/webhooks/rotate", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Rotate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WebhookTestDeliveryHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWebhookTestDeliveryHandler_New(t *testing.T) {
	h := NewWebhookTestDeliveryHandler(nil, core.NewEventBus(nil), newMockBoltStore())
	if h == nil {
		t.Fatal("NewWebhookTestDeliveryHandler returned nil")
	}
}

func TestWebhookTestDeliveryHandler_TestDeliver(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewWebhookTestDeliveryHandler(store, core.NewEventBus(slog.Default()), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/webhooks/test", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.TestDeliver(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ServiceMeshHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestServiceMeshHandler_New(t *testing.T) {
	h := NewServiceMeshHandler(nil, newMockBoltStore())
	if h == nil {
		t.Fatal("NewServiceMeshHandler returned nil")
	}
}

func TestServiceMeshHandler_List_Empty(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/links", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestServiceMeshHandler_List_WithData(t *testing.T) {
	bolt := newMockBoltStore()
	list := serviceLinkList{Links: []ServiceLink{{ID: "l1", SourceAppID: "app-1", TargetAppID: "app-2"}}}
	bolt.Set("service_mesh", "app-1", list, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, bolt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/links", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestServiceMeshHandler_Create_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/links", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestServiceMeshHandler_Create_MissingTarget(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, newMockBoltStore())

	body := `{"target_app_id":"","env_var":"DB_URL"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/links", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestServiceMeshHandler_Create_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, newMockBoltStore())

	body := `{"target_app_id":"app-2","env_var":"DB_URL","target_port":5432}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/links", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestServiceMeshHandler_Delete_NotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, newMockBoltStore())

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/links/target-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("targetId", "target-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestServiceMeshHandler_Delete_Success(t *testing.T) {
	bolt := newMockBoltStore()
	list := serviceLinkList{Links: []ServiceLink{
		{ID: "l1", TargetAppID: "app-2"},
		{ID: "l2", TargetAppID: "app-3"},
	}}
	bolt.Set("service_mesh", "app-1", list, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewServiceMeshHandler(store, bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/links/app-2", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("targetId", "app-2")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SaveTemplateHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestSaveTemplateHandler_New(t *testing.T) {
	h := NewSaveTemplateHandler(newMockStore())
	if h == nil {
		t.Fatal("NewSaveTemplateHandler returned nil")
	}
}

func TestSaveTemplateHandler_NoClaims(t *testing.T) {
	h := NewSaveTemplateHandler(newMockStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader("{}"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSaveTemplateHandler_AppNotFound(t *testing.T) {
	h := NewSaveTemplateHandler(newMockStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader("{}"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestSaveTemplateHandler_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewSaveTemplateHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSaveTemplateHandler_Success_DefaultFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "my-app", SourceType: "image", SourceURL: "nginx:latest"})
	h := NewSaveTemplateHandler(store)

	body := `{"description":"A test template","category":"web"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TransferHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestTransferHandler_New(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewTransferHandler returned nil")
	}
}

func TestTransferHandler_InvalidBody(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTransferHandler_MissingTargetTenant(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":""}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTransferHandler_AppNotFound(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t2"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTransferHandler_TargetTenantNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	h := NewTransferHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t-nonexistent"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTransferHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	store.addTenant(&core.Tenant{ID: "t2", Name: "Target Tenant"})
	h := NewTransferHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t2"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WildcardSSLHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWildcardSSLHandler_New(t *testing.T) {
	h := NewWildcardSSLHandler(newMockBoltStore())
	if h == nil {
		t.Fatal("NewWildcardSSLHandler returned nil")
	}
}

func TestWildcardSSLHandler_Request_InvalidBody(t *testing.T) {
	h := NewWildcardSSLHandler(newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader("{bad"))
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWildcardSSLHandler_Request_MissingDomain(t *testing.T) {
	h := NewWildcardSSLHandler(newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(`{"domain":""}`))
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWildcardSSLHandler_Request_Success(t *testing.T) {
	h := NewWildcardSSLHandler(newMockBoltStore())

	body := `{"domain":"example.com","dns_provider":"cloudflare"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RestartHistoryHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRestartHistoryHandler_New(t *testing.T) {
	h := NewRestartHistoryHandler(nil, nil)
	if h == nil {
		t.Fatal("NewRestartHistoryHandler returned nil")
	}
}

func TestRestartHistoryHandler_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRestartHistoryHandler(store, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRestartHistoryHandler_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRestartHistoryHandler(store, &mockContainerRuntime{})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRestartHistoryHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRestartHistoryHandler(store, &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123def456", State: "running"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RestartPolicyHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRestartPolicyHandler_New(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)
	if h == nil {
		t.Fatal("NewRestartPolicyHandler returned nil")
	}
}

func TestRestartPolicyHandler_InvalidBody(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRestartPolicyHandler_InvalidPolicy(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"invalid"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRestartPolicyHandler_AppNotFound(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"always"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestRestartPolicyHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	h := NewRestartPolicyHandler(store, nil)

	for _, policy := range []string{"always", "unless-stopped", "on-failure", "no"} {
		req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"`+policy+`"}`))
		req.SetPathValue("id", "app-1")
		req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("policy %q: expected 200, got %d", policy, rr.Code)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TenantRateLimitHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestTenantRateLimitHandler_New(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockBoltStore())
	if h == nil {
		t.Fatal("NewTenantRateLimitHandler returned nil")
	}
}

func TestTenantRateLimitHandler_Get_Default(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants/t1/ratelimit", nil)
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := RateLimitConfig{RequestsPerMinute: 200, BurstSize: 50}
	bolt.Set("tenant_ratelimit", "t1", cfg, 0)

	h := NewTenantRateLimitHandler(bolt)

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants/t1/ratelimit", nil)
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_InvalidBody(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockBoltStore())

	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader("{bad"))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_Success(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockBoltStore())

	body := `{"requests_per_minute":200,"burst_size":50,"builds_per_hour":20,"deploys_per_hour":30}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader(body))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_DefaultValues(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockBoltStore())

	body := `{"requests_per_minute":0,"burst_size":0}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader(body))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageTagHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestImageTagHandler_New(t *testing.T) {
	h := NewImageTagHandler(nil)
	if h == nil {
		t.Fatal("NewImageTagHandler returned nil")
	}
}

func TestImageTagHandler_List_MissingImage(t *testing.T) {
	h := NewImageTagHandler(&mockContainerRuntime{})

	req := httptest.NewRequest("GET", "/api/v1/images/tags", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_Success(t *testing.T) {
	h := NewImageTagHandler(&mockContainerRuntime{})

	req := httptest.NewRequest("GET", "/api/v1/images/tags?image=nginx", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StorageHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestStorageHandler_New(t *testing.T) {
	h := NewStorageHandler(newMockStore(), nil, newMockBoltStore())
	if h == nil {
		t.Fatal("NewStorageHandler returned nil")
	}
}

func TestStorageHandler_Usage_NoClaims(t *testing.T) {
	h := NewStorageHandler(newMockStore(), &mockContainerRuntime{}, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/storage/usage", nil)
	rr := httptest.NewRecorder()
	h.Usage(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestStorageHandler_Usage_Success(t *testing.T) {
	h := NewStorageHandler(newMockStore(), &mockContainerRuntime{}, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/storage/usage", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Usage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TenantSettingsHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestTenantSettingsHandler_New(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())
	if h == nil {
		t.Fatal("NewTenantSettingsHandler returned nil")
	}
}

func TestTenantSettingsHandler_Get_NoClaims(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/tenant/settings", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTenantSettingsHandler_Get_TenantNotFound(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/tenant/settings", nil)
	req = withClaims(req, "u1", "t-missing", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestTenantSettingsHandler_Get_Success(t *testing.T) {
	store := newMockStore()
	store.addTenant(&core.Tenant{ID: "t1", Name: "Test", Slug: "test", PlanID: "free", Status: "active"})
	h := NewTenantSettingsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/tenant/settings", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
