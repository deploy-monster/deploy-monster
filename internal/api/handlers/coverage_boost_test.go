package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// DiskUsageHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestDiskUsageHandler_AppDisk_NilRuntime(t *testing.T) {
	h := NewDiskUsageHandler(nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/disk", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.AppDisk(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if body["app_id"] != "app-1" {
		t.Errorf("app_id = %v, want app-1", body["app_id"])
	}
}

func TestDiskUsageHandler_AppDisk_WithRuntime(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "c1", State: "running"}},
	}
	h := NewDiskUsageHandler(runtime)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-2/disk", nil)
	req.SetPathValue("id", "app-2")
	rr := httptest.NewRecorder()
	h.AppDisk(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if body["containers"] != float64(1) {
		t.Errorf("containers = %v, want 1", body["containers"])
	}
}

func TestDiskUsageHandler_SystemDisk(t *testing.T) {
	h := NewDiskUsageHandler(nil)

	req := httptest.NewRequest("GET", "/api/v1/admin/disk", nil)
	rr := httptest.NewRecorder()
	h.SystemDisk(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ErrorPageHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestErrorPageHandler_Get(t *testing.T) {
	store := newMockStore()
	h := NewErrorPageHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/error-pages", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestErrorPageHandler_Update(t *testing.T) {
	store := newMockStore()
	h := NewErrorPageHandler(store)

	body := `{"page_502":"<h1>Down</h1>","page_503":"<h1>Unavailable</h1>"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "updated" {
		t.Errorf("status = %v, want updated", resp["status"])
	}
}

func TestErrorPageHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewErrorPageHandler(store)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader("not json"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageCleanupHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestImageCleanupHandler_DanglingImages(t *testing.T) {
	runtime := &mockContainerRuntime{}
	h := NewImageCleanupHandler(runtime)

	req := httptest.NewRequest("GET", "/api/v1/images/dangling", nil)
	rr := httptest.NewRecorder()
	h.DanglingImages(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestImageCleanupHandler_Prune(t *testing.T) {
	runtime := &mockContainerRuntime{}
	h := NewImageCleanupHandler(runtime)

	req := httptest.NewRequest("DELETE", "/api/v1/images/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "pruned" {
		t.Errorf("status = %v, want pruned", resp["status"])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LogRetentionHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestLogRetentionHandler_Get(t *testing.T) {
	store := newMockStore()
	h := NewLogRetentionHandler(store, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/log-retention", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var cfg LogRetentionConfig
	json.NewDecoder(rr.Body).Decode(&cfg)
	if cfg.MaxSizeMB != 50 {
		t.Errorf("MaxSizeMB = %d, want 50", cfg.MaxSizeMB)
	}
	if cfg.MaxFiles != 5 {
		t.Errorf("MaxFiles = %d, want 5", cfg.MaxFiles)
	}
	if cfg.Driver != "json-file" {
		t.Errorf("Driver = %q, want json-file", cfg.Driver)
	}
}

func TestLogRetentionHandler_Update(t *testing.T) {
	store := newMockStore()
	h := NewLogRetentionHandler(store, newMockBoltStore())

	body := `{"max_size_mb":100,"max_files":10,"driver":"local"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_DefaultValues(t *testing.T) {
	store := newMockStore()
	h := NewLogRetentionHandler(store, newMockBoltStore())

	// max_size_mb <= 0 should default to 50, max_files <= 0 should default to 5
	body := `{"max_size_mb":-1,"max_files":0}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewLogRetentionHandler(store, newMockBoltStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader("{invalid"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// MaintenanceHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestMaintenanceHandler_Get(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/maintenance", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var cfg MaintenanceConfig
	json.NewDecoder(rr.Body).Decode(&cfg)
	if cfg.Enabled {
		t.Error("default maintenance mode should be disabled")
	}
}

func TestMaintenanceHandler_Update_Enable(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockBoltStore())

	body := `{"enabled":true,"message":"We are upgrading","allowed_ips":["10.0.0.1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "enabled" {
		t.Errorf("status = %v, want enabled", resp["status"])
	}
}

func TestMaintenanceHandler_Update_Disable(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockBoltStore())

	body := `{"enabled":false}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "disabled" {
		t.Errorf("status = %v, want disabled", resp["status"])
	}
}

func TestMaintenanceHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader("bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PortHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestPortHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test-app"})
	h := NewPortHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/ports", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPortHandler_Get_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewPortHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/ports", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestPortHandler_Update(t *testing.T) {
	store := newMockStore()
	h := NewPortHandler(store)

	body := `[{"container_port":8080,"protocol":"tcp","exposed":true}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPortHandler_Update_InvalidPort(t *testing.T) {
	store := newMockStore()
	h := NewPortHandler(store)

	body := `[{"container_port":-1,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPortHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewPortHandler(store)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader("not-json"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPortHandler_Update_PortOverMax(t *testing.T) {
	store := newMockStore()
	h := NewPortHandler(store)

	body := `[{"container_port":70000,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for port > 65535, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LabelsHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestLabelsHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", LabelsJSON: `{"env":"prod","team":"backend"}`})
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/labels", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLabelsHandler_Get_Empty(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test"})
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/labels", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLabelsHandler_Get_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/labels", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test"})
	h := NewLabelsHandler(store)

	body := `{"env":"staging","version":"v2"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewLabelsHandler(store)

	body := `{"env":"staging"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/missing/labels", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader("bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// IPWhitelistHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestIPWhitelistHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test"})
	h := NewIPWhitelistHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/access", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestIPWhitelistHandler_Get_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewIPWhitelistHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/access", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestIPWhitelistHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test"})
	h := NewIPWhitelistHandler(store)

	body := `{"enabled":true,"allowed_ips":["192.168.1.0/24"],"deny_ips":["10.0.0.1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/access", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestIPWhitelistHandler_Update_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewIPWhitelistHandler(store)

	body := `{"enabled":true}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/missing/access", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestIPWhitelistHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewIPWhitelistHandler(store)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/access", strings.NewReader("bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CommitRollbackHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestCommitRollbackHandler_RollbackToCommit_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "testapp", TenantID: "t1", Status: "running"})
	store.addDeployment("app-1", core.Deployment{
		AppID: "app-1", Version: 1, CommitSHA: "abc1234567890", Image: "myapp:v1",
	})
	store.addDeployment("app-1", core.Deployment{
		AppID: "app-1", Version: 2, CommitSHA: "def4567890abc", Image: "myapp:v2",
	})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"abc1234567890"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["version"] != float64(1) {
		t.Errorf("version = %v, want 1", resp["version"])
	}
}

func TestCommitRollbackHandler_RollbackToCommit_PartialMatch(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "testapp", TenantID: "t1", Status: "running"})
	store.addDeployment("app-1", core.Deployment{
		AppID: "app-1", Version: 3, CommitSHA: "abcdef1234567890", Image: "myapp:v3",
	})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"abcdef1"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_NotFound(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_EmptyCommit(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":""}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_InvalidBody(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListDeploymentsByApp = core.ErrNotFound
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"abc1234"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EnvCompareHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestEnvCompareHandler_Compare(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-left",
		Name:       "left-app",
		EnvVarsEnc: `[{"key":"DB_HOST","value":"localhost"},{"key":"DB_PORT","value":"5432"}]`,
	})
	store.addApp(&core.Application{
		ID:         "app-right",
		Name:       "right-app",
		EnvVarsEnc: `[{"key":"DB_HOST","value":"prod-server"},{"key":"REDIS_URL","value":"redis://localhost"}]`,
	})
	h := NewEnvCompareHandler(store)

	body := `{"left_app_id":"app-left","right_app_id":"app-right"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/env/compare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Compare(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	total := resp["total"].(float64)
	if total < 2 {
		t.Errorf("expected at least 2 diffs, got %v", total)
	}
}

func TestEnvCompareHandler_Compare_LeftNotFound(t *testing.T) {
	store := newMockStore()
	h := NewEnvCompareHandler(store)

	body := `{"left_app_id":"missing","right_app_id":"also-missing"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/env/compare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Compare(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestEnvCompareHandler_Compare_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewEnvCompareHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/env/compare", strings.NewReader("bad"))
	rr := httptest.NewRecorder()
	h.Compare(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EnvImportHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestEnvImportHandler_Import_DotEnv(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test"})
	h := NewEnvImportHandler(store)

	envContent := "DB_HOST=localhost\nDB_PORT=5432\n# comment\nREDIS_URL=\"redis://localhost\"\n"
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(envContent))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["imported"] != float64(3) {
		t.Errorf("imported = %v, want 3", resp["imported"])
	}
}

func TestEnvImportHandler_Import_JSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test"})
	h := NewEnvImportHandler(store)

	body := `[{"key":"API_KEY","value":"secret123"},{"key":"NODE_ENV","value":"production"}]`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Import_Empty(t *testing.T) {
	store := newMockStore()
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(""))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Import_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewEnvImportHandler(store)

	body := "KEY=VALUE\n"
	req := httptest.NewRequest("POST", "/api/v1/apps/missing/env/import", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Export_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/env/export", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Export_DotEnv(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		Name:       "test",
		EnvVarsEnc: `[{"key":"FOO","value":"bar"},{"key":"BAZ","value":"qux"}]`,
	})
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/env/export", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", rr.Header().Get("Content-Type"))
	}
	body := rr.Body.String()
	if !strings.Contains(body, "FOO=bar") {
		t.Errorf("expected FOO=bar in output, got %q", body)
	}
}

func TestEnvImportHandler_Export_JSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		Name:       "test",
		EnvVarsEnc: `[{"key":"FOO","value":"bar"}]`,
	})
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/env/export?format=json", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImportExportHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestImportExportHandler_Export(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app-1", Name: "my-web-app", Type: "service",
		SourceType: "image", SourceURL: "nginx:latest", Replicas: 2,
	})
	store.domainsByApp["app-1"] = []core.Domain{
		{FQDN: "example.com"},
		{FQDN: "www.example.com"},
	}
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/export", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var manifest AppManifest
	json.NewDecoder(rr.Body).Decode(&manifest)
	if manifest.Name != "my-web-app" {
		t.Errorf("name = %q, want my-web-app", manifest.Name)
	}
	if len(manifest.Domains) != 2 {
		t.Errorf("domains = %d, want 2", len(manifest.Domains))
	}
}

func TestImportExportHandler_Export_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/export", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import(t *testing.T) {
	store := newMockStore()
	store.projects["tenant-1"] = []core.Project{{ID: "proj-1", Name: "Default"}}
	h := NewImportExportHandler(store)

	manifest := `{"version":"1","name":"imported-app","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1,"domains":["imported.example.com"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(manifest))
	req = withClaims(req, "user-1", "tenant-1", "admin", "user@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import_NoClaims(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import_InvalidManifest(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader("bad"))
	req = withClaims(req, "user-1", "tenant-1", "admin", "user@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// DNSRecordHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestDNSRecordHandler_List(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=example.com", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_MissingFields(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_InvalidBody(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("POST", "/api/v1/dns/records", strings.NewReader("bad"))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_NoProvider(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	body := `{"name":"test","value":"1.2.3.4","type":"A"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Delete_NoProvider(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("DELETE", "/api/v1/dns/records/rec-1", nil)
	req.SetPathValue("id", "rec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// DomainVerifyHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestDomainVerifyHandler_Verify_EmptyFQDN(t *testing.T) {
	store := newMockStore()
	h := NewDomainVerifyHandler(store)

	body := `{"fqdn":""}`
	req := httptest.NewRequest("POST", "/api/v1/domains/dom-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "dom-1")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_Verify_WithFQDN(t *testing.T) {
	store := newMockStore()
	h := NewDomainVerifyHandler(store)

	body := `{"fqdn":"localhost"}`
	req := httptest.NewRequest("POST", "/api/v1/domains/dom-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "dom-1")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_BatchVerify(t *testing.T) {
	store := newMockStore()
	h := NewDomainVerifyHandler(store)

	body := `{"fqdns":["localhost"]}`
	req := httptest.NewRequest("POST", "/api/v1/domains/verify-batch", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.BatchVerify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_BatchVerify_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewDomainVerifyHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/domains/verify-batch", strings.NewReader("bad"))
	rr := httptest.NewRecorder()
	h.BatchVerify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// helpers — additional coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestWriteJSON_StatusAccepted(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusAccepted, map[string]string{"key": "value"})

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %q", rr.Header().Get("Content-Type"))
	}
}

func TestWriteError_NotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusNotFound, "not found")

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "not found" {
		t.Errorf("error = %q, want 'not found'", resp["error"])
	}
}
