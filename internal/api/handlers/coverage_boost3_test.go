package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// LicenseHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestLicenseHandler_New(t *testing.T) {
	h := NewLicenseHandler(newMockBoltStore())
	if h == nil {
		t.Fatal("NewLicenseHandler returned nil")
	}
}

func TestLicenseHandler_Get_Default(t *testing.T) {
	h := NewLicenseHandler(newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/admin/license", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLicenseHandler_Get_Stored(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("license", "current", LicenseInfo{Type: "pro", Status: "active"}, 0)
	h := NewLicenseHandler(bolt)
	req := httptest.NewRequest("GET", "/api/v1/admin/license", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLicenseHandler_Activate_InvalidBody(t *testing.T) {
	h := NewLicenseHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/admin/license", strings.NewReader("{bad"))
	rr := httptest.NewRecorder()
	h.Activate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestLicenseHandler_Activate_EmptyKey(t *testing.T) {
	h := NewLicenseHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/admin/license", strings.NewReader(`{"key":""}`))
	rr := httptest.NewRecorder()
	h.Activate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestLicenseHandler_Activate_ShortKey(t *testing.T) {
	h := NewLicenseHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/admin/license", strings.NewReader(`{"key":"abc"}`))
	rr := httptest.NewRecorder()
	h.Activate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestLicenseHandler_Activate_Success(t *testing.T) {
	h := NewLicenseHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/admin/license", strings.NewReader(`{"key":"enterprise-license-key-12345678"}`))
	rr := httptest.NewRecorder()
	h.Activate(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LogDownloadHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestLogDownloadHandler_New(t *testing.T) {
	h := NewLogDownloadHandler(nil)
	if h == nil {
		t.Fatal("NewLogDownloadHandler returned nil")
	}
}

func TestLogDownloadHandler_NilRuntime(t *testing.T) {
	h := NewLogDownloadHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/logs/download", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Download(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestLogDownloadHandler_NoContainer(t *testing.T) {
	h := NewLogDownloadHandler(&mockContainerRuntime{})
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/logs/download", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Download(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestLogDownloadHandler_Success(t *testing.T) {
	h := NewLogDownloadHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc12345", State: "running"}},
		logsData:   "line1\nline2\nline3\n",
	})
	req := httptest.NewRequest("GET", "/api/v1/apps/app-12345678/logs/download", nil)
	req.SetPathValue("id", "app-12345678")
	rr := httptest.NewRecorder()
	h.Download(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Disposition"), "attachment") {
		t.Error("expected Content-Disposition attachment header")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RegistryHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRegistryHandler_New(t *testing.T) {
	h := NewRegistryHandler(newMockBoltStore())
	if h == nil {
		t.Fatal("NewRegistryHandler returned nil")
	}
}

func TestRegistryHandler_List(t *testing.T) {
	h := NewRegistryHandler(newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/registries", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRegistryHandler_Add_NoClaims(t *testing.T) {
	h := NewRegistryHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/registries", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRegistryHandler_Add_InvalidBody(t *testing.T) {
	h := NewRegistryHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/registries", strings.NewReader("{bad"))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRegistryHandler_Add_MissingFields(t *testing.T) {
	h := NewRegistryHandler(newMockBoltStore())
	req := httptest.NewRequest("POST", "/api/v1/registries", strings.NewReader(`{"name":""}`))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRegistryHandler_Add_Success(t *testing.T) {
	h := NewRegistryHandler(newMockBoltStore())
	body := `{"name":"My Registry","url":"registry.example.com","username":"user","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/v1/registries", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SnapshotHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestSnapshotHandler_New(t *testing.T) {
	h := NewSnapshotHandler(newMockStore(), nil, core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewSnapshotHandler returned nil")
	}
}

func TestSnapshotHandler_Create_NoClaims(t *testing.T) {
	h := NewSnapshotHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/snapshots", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSnapshotHandler_Create_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-12345678", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewSnapshotHandler(store, nil, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("POST", "/api/v1/apps/app-12345678/snapshots", nil)
	req.SetPathValue("id", "app-12345678")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestSnapshotHandler_List(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewSnapshotHandler(store, nil, core.NewEventBus(nil))
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/snapshots", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// UsageHistoryHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestUsageHistoryHandler_New(t *testing.T) {
	h := NewUsageHistoryHandler(newMockBoltStore())
	if h == nil {
		t.Fatal("NewUsageHistoryHandler returned nil")
	}
}

func TestUsageHistoryHandler_NoClaims(t *testing.T) {
	h := NewUsageHistoryHandler(newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/billing/usage/history", nil)
	rr := httptest.NewRecorder()
	h.Hourly(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestUsageHistoryHandler_Default24h(t *testing.T) {
	h := NewUsageHistoryHandler(newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/billing/usage/history", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Hourly(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestUsageHistoryHandler_7d(t *testing.T) {
	h := NewUsageHistoryHandler(newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/billing/usage/history?period=7d", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Hourly(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestUsageHistoryHandler_30d(t *testing.T) {
	h := NewUsageHistoryHandler(newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/billing/usage/history?period=30d", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Hourly(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestUsageHistoryHandler_StoredData(t *testing.T) {
	bolt := newMockBoltStore()
	data := usageHistory{Buckets: []UsageBucket{{Hour: "2025-01-01T00:00", CPUSeconds: 100}}}
	bolt.Set("usage_history", "t1:24h", data, 0)

	h := NewUsageHistoryHandler(bolt)
	req := httptest.NewRequest("GET", "/api/v1/billing/usage/history", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Hourly(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// InviteHandler — List (was 0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestInviteHandler_List_NoClaims(t *testing.T) {
	h := NewInviteHandler(newMockStore(), core.NewEventBus(nil))
	req := httptest.NewRequest("GET", "/api/v1/team/invites", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestInviteHandler_List_Success(t *testing.T) {
	h := NewInviteHandler(newMockStore(), core.NewEventBus(nil))
	req := httptest.NewRequest("GET", "/api/v1/team/invites", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestInviteHandler_List_Error(t *testing.T) {
	store := newMockStore()
	store.errListInvitesByTenant = core.ErrNotFound
	h := NewInviteHandler(store, core.NewEventBus(nil))
	req := httptest.NewRequest("GET", "/api/v1/team/invites", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// BackupHandler — Download (was 0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestBackupHandler_Download_NilStorage(t *testing.T) {
	h := NewBackupHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("GET", "/api/v1/backups/bak-1/download", nil)
	req.SetPathValue("key", "bak-1")
	rr := httptest.NewRecorder()
	h.Download(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
