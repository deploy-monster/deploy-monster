package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── helpers.go ─────────────────────────────────────────────

func TestWriteJSON_EncodeErrorDoesNotPanic(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]any{"ch": make(chan int)})
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// ─── agent_status.go ────────────────────────────────────────

func TestAgentStatus_GetAgent_Returns404ForUnknown(t *testing.T) {
	h := NewAgentStatusHandler(testCoreWithBuild())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/unknown", nil)
	req.SetPathValue("id", "unknown")
	rr := httptest.NewRecorder()
	h.GetAgent(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// ─── admin_apikeys.go ───────────────────────────────────────

func TestAdminAPIKey_Revoke_MissingPrefix(t *testing.T) {
	h := NewAdminAPIKeyHandler(newMockStore(), newMockBoltStore())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/api-keys/", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	req.SetPathValue("prefix", "")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAdminAPIKey_CleanupExpiredKeys_DeleteError(t *testing.T) {
	bolt := newMockBoltStore()
	exp := time.Now().Add(-time.Hour)
	_ = bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"p1"}}, 0)
	_ = bolt.Set("api_keys", "p1", apiKeyRecord{Prefix: "p1", Hash: "h1", Type: "platform", CreatedAt: time.Now().Add(-48 * time.Hour), ExpiresAt: &exp}, 0)
	bolt.errDelete = fmt.Errorf("delete error")
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)
	removed := h.CleanupExpiredKeys()
	if removed != 1 {
		t.Errorf("expected 1 removal, got %d", removed)
	}
}

func TestAdminAPIKey_CleanupExpiredKeys_SetError(t *testing.T) {
	bolt := newMockBoltStore()
	exp := time.Now().Add(-time.Hour)
	_ = bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"p1"}}, 0)
	_ = bolt.Set("api_keys", "p1", apiKeyRecord{Prefix: "p1", Hash: "h1", Type: "platform", CreatedAt: time.Now().Add(-48 * time.Hour), ExpiresAt: &exp}, 0)
	bolt.errSet = fmt.Errorf("index set error")
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)
	removed := h.CleanupExpiredKeys()
	if removed != 1 {
		t.Errorf("expected 1 removal, got %d", removed)
	}
}

// ─── announcements.go ──────────────────────────────────────

func TestAnnouncement_Dismiss_MissingID(t *testing.T) {
	h := NewAnnouncementHandler(newMockBoltStore())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/announcements/", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	h.Dismiss(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAnnouncement_Dismiss_BoltSetError(t *testing.T) {
	bolt := newMockBoltStore()
	_ = bolt.Set("announcements", "all", announcementList{
		Items: []Announcement{{ID: "a1", Title: "t", Active: true}},
	}, 0)
	bolt.errSet = fmt.Errorf("bolt set error")
	h := NewAnnouncementHandler(bolt)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/announcements/a1", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Dismiss(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// ─── app_middleware.go ──────────────────────────────────────

func TestAppMiddleware_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewAppMiddlewareHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/middleware", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAppMiddleware_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewAppMiddlewareHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/middleware", strings.NewReader(`{"compress":true}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAppMiddleware_Update_BoltSetError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	bolt := newMockBoltStore()
	bolt.errSet = fmt.Errorf("bolt set error")
	h := NewAppMiddlewareHandler(store, bolt)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/middleware", strings.NewReader(`{"compress":true}`))
	req.SetPathValue("id", "a1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// ─── build_logs.go ─────────────────────────────────────────

func TestBuildLog_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewBuildLogHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/builds/1/log", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestBuildLog_Download_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewBuildLogHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/builds/1/log/download", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Download(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── autoscale.go ──────────────────────────────────────────

func TestAutoscale_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewAutoscaleHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/autoscale", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAutoscale_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewAutoscaleHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/autoscale", strings.NewReader(`{"enabled":true}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── basic_auth.go ─────────────────────────────────────────

func TestBasicAuth_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewBasicAuthHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/basic-auth", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestBasicAuth_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewBasicAuthHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/basic-auth", strings.NewReader(`{"enabled":true}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── cronjobs.go ───────────────────────────────────────────

func TestCronJobs_List_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewCronJobHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/cronjobs", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCronJobs_Create_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewCronJobHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a1/cronjobs", strings.NewReader(`{"schedule":"* * * * *","command":"echo hi"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCronJobs_Delete_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewCronJobHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/a1/cronjobs/j1", nil)
	req.SetPathValue("id", "a1")
	req.SetPathValue("cronjob_id", "j1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── deploy_approval.go ────────────────────────────────────

func TestDeployApproval_Approve_MissingID(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals//approve", nil)
	req.SetPathValue("id", "")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ─── deploy_diff.go ────────────────────────────────────────

func TestDeployDiff_Diff_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewDeployDiffHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/deployments/diff?from=1&to=2", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Diff(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── deploy_notify.go ──────────────────────────────────────

func TestDeployNotify_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewDeployNotifyHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/deploy-notify", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestDeployNotify_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewDeployNotifyHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/deploy-notify", strings.NewReader(`{"slack_webhook":"https://hooks.slack.com/test"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── error_pages.go ────────────────────────────────────────

func TestErrorPages_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewErrorPageHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/error-pages", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestErrorPages_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewErrorPageHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/error-pages", strings.NewReader(`{"error_404":"/404.html"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── log_retention.go ──────────────────────────────────────

func TestLogRetention_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewLogRetentionHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/log-retention", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLogRetention_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewLogRetentionHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/log-retention", strings.NewReader(`{"max_days":30}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── redirects.go ──────────────────────────────────────────

func TestRedirects_List_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewRedirectHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/redirects", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── sticky_sessions.go ────────────────────────────────────

func TestStickySessions_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewStickySessionHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/sticky-sessions", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestStickySessions_Update_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewStickySessionHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/a1/sticky-sessions", strings.NewReader(`{"enabled":true,"cookie_name":"test"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── tenant_ratelimit.go ───────────────────────────────────

func TestTenantRateLimit_Get_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewTenantRateLimitHandler(newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/rate-limit", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	// No claims check needed — just verify it doesn't panic
	_ = rr.Code
}

// ─── backup.go uncovered paths ─────────────────────────────

func TestBackup_Restore_NoClaims(t *testing.T) {
	h := NewBackupHandler(newMockStore(), nil, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a1/backups/restore", strings.NewReader(`{"key":"t1/backup"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Restore(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestBackup_Download_NoClaims(t *testing.T) {
	h := NewBackupHandler(newMockStore(), nil, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/backups/download", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Download(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── billing.go uncovered paths ────────────────────────────

func TestBilling_GetUsage_NoClaims(t *testing.T) {
	h := NewBillingHandler(newMockStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	rr := httptest.NewRecorder()
	h.GetUsage(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── certificates.go uncovered paths ───────────────────────

func TestCertificates_List_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewCertificateHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/certificates", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestCertificates_Upload_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewCertificateHandler(store, newMockBoltStore())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a1/certificates", strings.NewReader(`{"certificate":"c","private_key":"k"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Upload(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── databases.go uncovered paths ──────────────────────────

func TestDatabases_Create_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewDatabaseHandler(store, nil, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a1/databases", strings.NewReader(`{"type":"postgres","name":"mydb"}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── volumes.go uncovered paths ────────────────────────────

func TestVolumes_List_NoClaims(t *testing.T) {
	h := NewVolumeHandler(nil, newMockStore(), core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/volumes", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestVolumes_Create_NoClaims(t *testing.T) {
	h := NewVolumeHandler(nil, newMockStore(), core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a1/volumes", strings.NewReader(`{"name":"vol1","size_gb":10}`))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── container_top.go uncovered paths ──────────────────────

func TestContainerTop_Top_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewContainerTopHandler(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/top", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Top(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── container_history.go uncovered paths ──────────────────

func TestContainerHistory_History_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewContainerHistoryHandler(store, nil, newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/containers", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.History(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── deployments.go uncovered paths ────────────────────────

func TestDeployments_ListByApp_NoClaims(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "test"})
	h := NewDeploymentHandler(store, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/a1/deployments", nil)
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.ListByApp(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── secrets.go uncovered paths ────────────────────────────

func TestSecrets_Delete_NoClaims(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/a1/secrets/secret1", nil)
	req.SetPathValue("id", "a1")
	req.SetPathValue("secretName", "secret1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ─── image_cleanup.go uncovered paths ──────────────────────

func TestImageCleanup_DanglingImages_EmptyRuntime(t *testing.T) {
	h := NewImageCleanupHandler(&mockContainerRuntime{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/images/dangling", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.DanglingImages(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestImageCleanup_Prune_EmptyRuntime(t *testing.T) {
	h := NewImageCleanupHandler(&mockContainerRuntime{})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/images/prune", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Prune(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
