package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Ensure context import is used (needed by mock module interfaces).
var _ = context.Background

// =============================================================================
// AdminAPIKeyHandler — List, Generate, Revoke
// =============================================================================

func TestAdminAPIKeyHandler_List_EmptyIndex(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest("GET", "/api/v1/admin/api-keys", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
}

func TestAdminAPIKeyHandler_List_WithKeys(t *testing.T) {
	bolt := newMockBoltStore()
	// Seed an index with two prefixes
	bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"pfx-a", "pfx-b"}}, 0)
	bolt.Set("api_keys", "pfx-a", apiKeyRecord{Prefix: "pfx-a", Hash: "h1", Type: "platform", CreatedBy: "u1", CreatedAt: time.Now()}, 0)
	bolt.Set("api_keys", "pfx-b", apiKeyRecord{Prefix: "pfx-b", Hash: "h2", Type: "platform", CreatedBy: "u2", CreatedAt: time.Now()}, 0)

	h := NewAdminAPIKeyHandler(newMockStore(), bolt)
	req := httptest.NewRequest("GET", "/api/v1/admin/api-keys", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 2 {
		t.Errorf("expected total 2, got %v", resp["total"])
	}
}

func TestAdminAPIKeyHandler_List_MissingKeyRecord(t *testing.T) {
	bolt := newMockBoltStore()
	// Index has a prefix but the record doesn't exist
	bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"pfx-missing"}}, 0)

	h := NewAdminAPIKeyHandler(newMockStore(), bolt)
	req := httptest.NewRequest("GET", "/api/v1/admin/api-keys", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0 (record missing), got %v", resp["total"])
	}
}

func TestAdminAPIKeyHandler_Generate_Forbidden(t *testing.T) {
	h := NewAdminAPIKeyHandler(newMockStore(), newMockBoltStore())

	// No claims
	req := httptest.NewRequest("POST", "/api/v1/admin/api-keys", nil)
	rr := httptest.NewRecorder()
	h.Generate(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	// Non-super-admin claims
	req2 := httptest.NewRequest("POST", "/api/v1/admin/api-keys", nil)
	req2 = withClaims(req2, "u1", "t1", "role_admin", "a@b.com")
	rr2 := httptest.NewRecorder()
	h.Generate(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-super-admin, got %d", rr2.Code)
	}
}

func TestAdminAPIKeyHandler_Generate_Success(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest("POST", "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["key"] == nil || resp["key"].(string) == "" {
		t.Error("expected key to be returned")
	}
	if resp["prefix"] == nil {
		t.Error("expected prefix")
	}
}

func TestAdminAPIKeyHandler_Revoke_Success(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("api_keys", "pfx-x", apiKeyRecord{Prefix: "pfx-x"}, 0)
	bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"pfx-x", "pfx-y"}}, 0)

	h := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/api-keys/pfx-x", nil)
	req.SetPathValue("prefix", "pfx-x")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	// Verify index is updated
	var idx apiKeyIndex
	bolt.Get("api_keys", "_index", &idx)
	for _, p := range idx.Prefixes {
		if p == "pfx-x" {
			t.Error("pfx-x should have been removed from index")
		}
	}
}

func TestAdminAPIKeyHandler_Revoke_NoIndex(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/api-keys/pfx-z", nil)
	req.SetPathValue("prefix", "pfx-z")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)

	// Should still return 204 even if index doesn't exist
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// AgentStatusHandler — List, GetAgent
// =============================================================================

func TestAgentStatusHandler_List_WithContainer(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Name: "test-app"},
			{ID: "c2", Name: "test-app-2"},
		},
	}
	c.Services.Container = runtime

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if int(local["containers"].(float64)) != 2 {
		t.Errorf("expected 2 containers, got %v", local["containers"])
	}
}

func TestAgentStatusHandler_List_NilContainer(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	// c.Services.Container is nil

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAgentStatusHandler_List_ContainerListError(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	c.Services.Container = &mockContainerRuntime{listErr: io.EOF}

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 even with container list error, got %d", rr.Code)
	}
}

func TestAgentStatusHandler_List_DegradedHealth(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	// Register a module with degraded health
	c.Registry = core.NewRegistry()
	c.Registry.Register(&degradedModule{})

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if local["status"] != "degraded" {
		t.Errorf("expected degraded, got %v", local["status"])
	}
}

func TestAgentStatusHandler_List_DownHealth(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	c.Registry.Register(&downModule{})

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if local["status"] != "unhealthy" {
		t.Errorf("expected unhealthy, got %v", local["status"])
	}
}

func TestAgentStatusHandler_GetAgent_Local(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents/local", nil)
	req.SetPathValue("id", "local")
	rr := httptest.NewRecorder()
	h.GetAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAgentStatusHandler_GetAgent_Remote(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "2.0.0"}
	c.Registry = core.NewRegistry()

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents/remote-1", nil)
	req.SetPathValue("id", "remote-1")
	rr := httptest.NewRecorder()
	h.GetAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp AgentNodeStatus
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Status != "unknown" {
		t.Errorf("expected unknown, got %q", resp.Status)
	}
	if resp.ServerID != "remote-1" {
		t.Errorf("expected server_id remote-1, got %q", resp.ServerID)
	}
}

// =============================================================================
// PinHandler — Pin, Unpin
// =============================================================================

func TestPinHandler_Pin_NoClaims(t *testing.T) {
	h := NewPinHandler(newMockStore(), newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestPinHandler_Pin_NewPin(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewPinHandler(newMockStore(), bolt)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["pinned"] != "true" {
		t.Errorf("expected pinned=true, got %q", resp["pinned"])
	}
}

func TestPinHandler_Pin_AlreadyPinned(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("app_pins", "u1", pinnedApps{AppIDs: []string{"app-1"}}, 0)
	h := NewPinHandler(newMockStore(), bolt)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_NoClaims(t *testing.T) {
	h := NewPinHandler(newMockStore(), newMockBoltStore())
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_NoPins(t *testing.T) {
	h := NewPinHandler(newMockStore(), newMockBoltStore())
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_ExistingPin(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("app_pins", "u1", pinnedApps{AppIDs: []string{"app-1", "app-2"}}, 0)
	h := NewPinHandler(newMockStore(), bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Verify app-1 is removed but app-2 remains
	var pins pinnedApps
	bolt.Get("app_pins", "u1", &pins)
	if len(pins.AppIDs) != 1 || pins.AppIDs[0] != "app-2" {
		t.Errorf("expected [app-2], got %v", pins.AppIDs)
	}
}

// =============================================================================
// BackupHandler — Download
// =============================================================================

func TestBackupHandler_Download_NilStorage_Boost4(t *testing.T) {
	h := NewBackupHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("GET", "/api/v1/backups/test.tar/download", nil)
	req.SetPathValue("key", "test.tar")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestBackupHandler_Download_Success(t *testing.T) {
	storage := &mockBackupStorage{
		fileData: "backup-data-bytes",
	}
	h := NewBackupHandler(newMockStore(), storage, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("GET", "/api/v1/backups/test.tar/download", nil)
	req.SetPathValue("key", "test.tar")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("wrong Content-Type: %q", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Header().Get("Content-Disposition"), "test.tar") {
		t.Error("expected Content-Disposition to contain the filename")
	}
	if rr.Body.String() != "backup-data-bytes" {
		t.Errorf("body = %q, want backup-data-bytes", rr.Body.String())
	}
}

func TestBackupHandler_Download_NotFound(t *testing.T) {
	storage := &mockBackupStorage{errDown: io.EOF}
	h := NewBackupHandler(newMockStore(), storage, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("GET", "/api/v1/backups/missing.tar/download", nil)
	req.SetPathValue("key", "missing.tar")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// CertificateHandler — List (expired cert path), Upload
// =============================================================================

func TestCertificateHandler_List_ExpiredCerts(t *testing.T) {
	bolt := newMockBoltStore()
	// Seed with one expired cert
	bolt.Set("certificates", "all", certStore{
		Certs: []CertInfo{
			{ID: "c1", Domain: "example.com", ExpiresAt: time.Now().Add(-24 * time.Hour), Status: "active"},
		},
	}, 0)

	h := NewCertificateHandler(newMockStore(), bolt)
	req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	cert := data[0].(map[string]any)
	if cert["status"] != "expired" {
		t.Errorf("expected status 'expired', got %v", cert["status"])
	}
}

func TestCertificateHandler_Upload_MissingFields(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(`{"domain_id":"","cert_pem":"","key_pem":""}`))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCertificateHandler_Upload_InvalidCert(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates",
		strings.NewReader(`{"domain_id":"d1","cert_pem":"not-a-cert","key_pem":"not-a-key"}`))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid cert, got %d", rr.Code)
	}
}

// =============================================================================
// BuildCacheHandler — Stats, Clear
// =============================================================================

func TestBuildCacheHandler_Stats_NilRuntime(t *testing.T) {
	h := NewBuildCacheHandler(nil, newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBuildCacheHandler_Stats_WithImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"app:v1"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{"app:v2"}, Size: 200 * 1024 * 1024},
		},
	}
	bolt := newMockBoltStore()
	bolt.Set("buildcache", "stats", buildCacheStats{TotalBuilds: 10, CacheHits: 7, CacheMisses: 3, TotalSavedSec: 120}, 0)

	h := NewBuildCacheHandler(runtime, bolt)
	req := httptest.NewRequest("GET", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["layers"].(float64)) != 2 {
		t.Errorf("expected 2 layers, got %v", resp["layers"])
	}
	if int(resp["total_builds"].(float64)) != 10 {
		t.Errorf("expected total_builds 10, got %v", resp["total_builds"])
	}
}

func TestBuildCacheHandler_Stats_ImageListError(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewBuildCacheHandler(runtime, newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Stats(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestBuildCacheHandler_Clear_NilRuntime(t *testing.T) {
	h := NewBuildCacheHandler(nil, newMockBoltStore())
	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBuildCacheHandler_Clear_WithDanglingImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"<none>:<none>"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{"app:latest"}, Size: 50 * 1024 * 1024}, // Not dangling
			{ID: "img3", Tags: []string{}, Size: 75 * 1024 * 1024},             // No tags = dangling
		},
	}
	h := NewBuildCacheHandler(runtime, newMockBoltStore())
	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["images_removed"].(float64)) != 2 {
		t.Errorf("expected 2 images removed, got %v", resp["images_removed"])
	}
}

func TestBuildCacheHandler_Clear_ImageListError(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewBuildCacheHandler(runtime, newMockBoltStore())

	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ContainerHistoryHandler — History with stored metrics, period filters
// =============================================================================

func TestContainerHistoryHandler_History_WithStoredMetrics(t *testing.T) {
	bolt := newMockBoltStore()
	now := time.Now()
	bolt.Set("metrics_ring", "app-1", metricsRingData{
		Points: []ContainerResourcePoint{
			{Timestamp: now.Add(-30 * time.Minute), CPUPercent: 50.0, MemoryMB: 256},
			{Timestamp: now.Add(-10 * time.Minute), CPUPercent: 70.0, MemoryMB: 512},
		},
	}, 0)

	h := NewContainerHistoryHandler(nil, bolt)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=1h", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 2 {
		t.Errorf("expected 2 points, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_Period24h(t *testing.T) {
	bolt := newMockBoltStore()
	now := time.Now()
	bolt.Set("metrics_ring", "app-1", metricsRingData{
		Points: []ContainerResourcePoint{
			{Timestamp: now.Add(-48 * time.Hour), CPUPercent: 20.0}, // Outside 24h
			{Timestamp: now.Add(-12 * time.Hour), CPUPercent: 40.0}, // Inside 24h
		},
	}, 0)

	h := NewContainerHistoryHandler(nil, bolt)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=24h", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 point inside 24h, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_Period7d(t *testing.T) {
	bolt := newMockBoltStore()
	now := time.Now()
	bolt.Set("metrics_ring", "app-1", metricsRingData{
		Points: []ContainerResourcePoint{
			{Timestamp: now.Add(-3 * 24 * time.Hour), CPUPercent: 30.0},
		},
	}, 0)

	h := NewContainerHistoryHandler(nil, bolt)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=7d", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 point inside 7d, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_EmptyMetrics_24h(t *testing.T) {
	h := NewContainerHistoryHandler(nil, newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=24h", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 96 {
		t.Errorf("expected 96 points for 24h empty, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_EmptyMetrics_7d(t *testing.T) {
	h := NewContainerHistoryHandler(nil, newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=7d", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 168 {
		t.Errorf("expected 168 points for 7d empty, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_NilBolt(t *testing.T) {
	h := NewContainerHistoryHandler(nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AppMiddlewareHandler — Get stored, Update success
// =============================================================================

func TestAppMiddlewareHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := MiddlewareConfig{Compress: false, Headers: map[string]string{"X-Custom": "val"}}
	bolt.Set("app_middleware", "app-1", cfg, 0)

	h := NewAppMiddlewareHandler(newMockStore(), bolt)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/middleware", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp MiddlewareConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Headers["X-Custom"] != "val" {
		t.Errorf("expected X-Custom header, got %v", resp.Headers)
	}
}

func TestAppMiddlewareHandler_Update_Success(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAppMiddlewareHandler(newMockStore(), bolt)

	body := `{"compress":true,"headers":{"X-Frame-Options":"DENY"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/middleware", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AutoscaleHandler — Get stored, Update min/max correction
// =============================================================================

func TestAutoscaleHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := AutoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 8}
	bolt.Set("autoscale", "app-1", cfg, 0)

	h := NewAutoscaleHandler(newMockStore(), bolt)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/autoscale", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp AutoscaleConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.Enabled || resp.MinReplicas != 2 || resp.MaxReplicas != 8 {
		t.Errorf("unexpected config: %+v", resp)
	}
}

func TestAutoscaleHandler_Update_MinMaxCorrection(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewAutoscaleHandler(newMockStore(), bolt)

	// min=0 should be corrected to 1, max=0 should be corrected to min
	body := `{"enabled":true,"min_replicas":0,"max_replicas":0}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/autoscale", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// BasicAuthHandler — Get stored, Update default realm
// =============================================================================

func TestBasicAuthHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	cfg := BasicAuthConfig{Enabled: true, Realm: "Admin", Users: map[string]string{"admin": "hash123"}}
	bolt.Set("basic_auth", "app-1", cfg, 0)

	h := NewBasicAuthHandler(newMockStore(), bolt)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/basic-auth", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp BasicAuthConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Realm != "Admin" || !resp.Enabled {
		t.Errorf("unexpected config: %+v", resp)
	}
}

func TestBasicAuthHandler_Update_DefaultRealm(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewBasicAuthHandler(newMockStore(), bolt)

	body := `{"enabled":true,"realm":"","users":{"admin":"hash"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/basic-auth", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	// Verify the realm was defaulted to "Restricted"
	var stored BasicAuthConfig
	bolt.Get("basic_auth", "app-1", &stored)
	if stored.Realm != "Restricted" {
		t.Errorf("expected default realm 'Restricted', got %q", stored.Realm)
	}
}

// =============================================================================
// AdminHandler — SystemInfo, ListTenants
// =============================================================================

func TestAdminHandler_SystemInfo(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0", Commit: "abc123"}
	c.Registry = core.NewRegistry()

	h := NewAdminHandler(c, newMockStore())
	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()
	h.SystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["version"] != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %v", resp["version"])
	}
}

func TestAdminHandler_ListTenants_WithPagination(t *testing.T) {
	store := newMockStore()
	store.allTenantsList = []core.Tenant{
		{ID: "t1", Name: "Tenant1"},
		{ID: "t2", Name: "Tenant2"},
		{ID: "t3", Name: "Tenant3"},
	}

	h := NewAdminHandler(testCore(), store)
	req := httptest.NewRequest("GET", "/api/v1/admin/tenants?page=1&per_page=2", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(data))
	}
	if int(resp["total"].(float64)) != 3 {
		t.Errorf("expected total 3, got %v", resp["total"])
	}
}

func TestAdminHandler_ListTenants_Error(t *testing.T) {
	store := newMockStore()
	store.errListAllTenants = io.EOF

	h := NewAdminHandler(testCore(), store)
	req := httptest.NewRequest("GET", "/api/v1/admin/tenants", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestAdminHandler_ListTenants_DefaultPagination(t *testing.T) {
	store := newMockStore()
	h := NewAdminHandler(testCore(), store)

	// Zero/negative page and per_page should get defaults
	req := httptest.NewRequest("GET", "/api/v1/admin/tenants?page=0&per_page=-1", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAdminHandler_ListTenants_OverMaxPerPage(t *testing.T) {
	store := newMockStore()
	h := NewAdminHandler(testCore(), store)

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants?per_page=999", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// Mock helpers for this test file
// =============================================================================

// mockContainerRuntimeWithImages extends the mock to support ImageList/ImageRemove.
type mockContainerRuntimeWithImages struct {
	images       []core.ImageInfo
	imageListErr error
	removeErr    error
}

func (m *mockContainerRuntimeWithImages) Ping() error { return nil }
func (m *mockContainerRuntimeWithImages) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "c1", nil
}
func (m *mockContainerRuntimeWithImages) Stop(_ context.Context, _ string, _ int) error { return nil }
func (m *mockContainerRuntimeWithImages) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockContainerRuntimeWithImages) Restart(_ context.Context, _ string) error { return nil }
func (m *mockContainerRuntimeWithImages) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockContainerRuntimeWithImages) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (m *mockContainerRuntimeWithImages) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockContainerRuntimeWithImages) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *mockContainerRuntimeWithImages) ImagePull(_ context.Context, _ string) error { return nil }
func (m *mockContainerRuntimeWithImages) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	if m.imageListErr != nil {
		return nil, m.imageListErr
	}
	return m.images, nil
}
func (m *mockContainerRuntimeWithImages) ImageRemove(_ context.Context, id string) error {
	return m.removeErr
}
func (m *mockContainerRuntimeWithImages) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *mockContainerRuntimeWithImages) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// degradedModule is a module that reports HealthDegraded.
type degradedModule struct{}

func (d *degradedModule) ID() string                                       { return "test.degraded" }
func (d *degradedModule) Name() string                                     { return "Degraded" }
func (d *degradedModule) Version() string                                  { return "1.0.0" }
func (d *degradedModule) Dependencies() []string                           { return nil }
func (d *degradedModule) Init(_ context.Context, _ *core.Core) error       { return nil }
func (d *degradedModule) Start(_ context.Context) error                    { return nil }
func (d *degradedModule) Stop(_ context.Context) error                     { return nil }
func (d *degradedModule) Health() core.HealthStatus                        { return core.HealthDegraded }
func (d *degradedModule) Routes() []core.Route                             { return nil }
func (d *degradedModule) Events() []core.EventHandler                      { return nil }

// downModule is a module that reports HealthDown.
type downModule struct{}

func (d *downModule) ID() string                                       { return "test.down" }
func (d *downModule) Name() string                                     { return "Down" }
func (d *downModule) Version() string                                  { return "1.0.0" }
func (d *downModule) Dependencies() []string                           { return nil }
func (d *downModule) Init(_ context.Context, _ *core.Core) error       { return nil }
func (d *downModule) Start(_ context.Context) error                    { return nil }
func (d *downModule) Stop(_ context.Context) error                     { return nil }
func (d *downModule) Health() core.HealthStatus                        { return core.HealthDown }
func (d *downModule) Routes() []core.Route                             { return nil }
func (d *downModule) Events() []core.EventHandler                      { return nil }
