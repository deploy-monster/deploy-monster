package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// testCoreWithBuild returns a *core.Core with build info and a live registry.
func testCoreWithBuild() *core.Core {
	return &core.Core{
		Config: &core.Config{},
		Build: core.BuildInfo{
			Version: "1.0.0-test",
			Commit:  "abc123",
			Date:    "2025-01-01",
		},
		Registry: core.NewRegistry(),
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
}

// ─── SystemInfo ──────────────────────────────────────────────────────────────

func TestSystemInfo_ReturnsInfo(t *testing.T) {
	c := testCoreWithBuild()
	handler := NewAdminHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()

	handler.SystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["version"] != "1.0.0-test" {
		t.Errorf("expected version '1.0.0-test', got %v", resp["version"])
	}
	if resp["commit"] != "abc123" {
		t.Errorf("expected commit 'abc123', got %v", resp["commit"])
	}
	if resp["built"] != "2025-01-01" {
		t.Errorf("expected built '2025-01-01', got %v", resp["built"])
	}
	if resp["go"] != runtime.Version() {
		t.Errorf("expected go %q, got %v", runtime.Version(), resp["go"])
	}
	if resp["os"] != runtime.GOOS {
		t.Errorf("expected os %q, got %v", runtime.GOOS, resp["os"])
	}
	if resp["arch"] != runtime.GOARCH {
		t.Errorf("expected arch %q, got %v", runtime.GOARCH, resp["arch"])
	}

	// Verify memory section exists.
	mem, ok := resp["memory"].(map[string]any)
	if !ok {
		t.Fatal("expected memory object in response")
	}
	if _, exists := mem["alloc_mb"]; !exists {
		t.Error("expected alloc_mb in memory")
	}
	if _, exists := mem["sys_mb"]; !exists {
		t.Error("expected sys_mb in memory")
	}

	// Verify modules and events sections exist.
	if _, ok := resp["modules"]; !ok {
		t.Error("expected modules in response")
	}
	evts, ok := resp["events"].(map[string]any)
	if !ok {
		t.Fatal("expected events object in response")
	}
	if _, exists := evts["published"]; !exists {
		t.Error("expected published count in events")
	}
}

// ─── ListTenants ─────────────────────────────────────────────────────────────

func TestListTenants_ReturnsEmptyList(t *testing.T) {
	c := testCoreWithBuild()
	handler := NewAdminHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants", nil)
	rr := httptest.NewRecorder()

	handler.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected 0 tenants, got %d", len(data))
	}

	total, _ := resp["total"].(float64)
	if int(total) != 0 {
		t.Errorf("expected total 0, got %d", int(total))
	}
}

// ─── UpdateSettings ──────────────────────────────────────────────────────────

func TestUpdateSettings_Success(t *testing.T) {
	c := testCoreWithBuild()
	handler := NewAdminHandler(c)

	body := []byte(`{"auto_ssl": true, "backup_frequency": "daily"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/settings", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.UpdateSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "updated" {
		t.Errorf("expected status 'updated', got %v", resp["status"])
	}

	settings, ok := resp["settings"].(map[string]any)
	if !ok {
		t.Fatal("expected settings object in response")
	}
	if settings["auto_ssl"] != true {
		t.Errorf("expected auto_ssl true, got %v", settings["auto_ssl"])
	}
}

func TestUpdateSettings_InvalidJSON(t *testing.T) {
	c := testCoreWithBuild()
	handler := NewAdminHandler(c)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/settings", bytes.NewReader([]byte("bad json")))
	rr := httptest.NewRecorder()

	handler.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}
