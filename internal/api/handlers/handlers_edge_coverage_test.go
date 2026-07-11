package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	internalAuth "github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// admin_apikeys.go — CleanupExpiredKeys: index Set error path
// =============================================================================

func TestCleanupExpiredKeys_IndexSetError(t *testing.T) {
	bolt := newMockBoltStore()
	past := time.Now().Add(-2 * time.Hour)

	// Seed data before setting error
	_ = bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"exp1"}}, 0)
	_ = bolt.Set("api_keys", "exp1", apiKeyRecord{Prefix: "exp1", Hash: "h1", ExpiresAt: &past}, 0)

	// Set error on the index update after deletion
	bolt.errSet = fmt.Errorf("index write error")

	h := NewAdminAPIKeyHandler(nil, bolt)
	removed := h.CleanupExpiredKeys()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestCleanupExpiredKeys_KeyGetErrorSkips(t *testing.T) {
	bolt := newMockBoltStore()
	past := time.Now().Add(-2 * time.Hour)

	_ = bolt.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"exp1", "missing_key"}}, 0)
	_ = bolt.Set("api_keys", "exp1", apiKeyRecord{Prefix: "exp1", Hash: "h1", ExpiresAt: &past}, 0)

	h := NewAdminAPIKeyHandler(nil, bolt)
	removed := h.CleanupExpiredKeys()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

// =============================================================================
// admin_apikeys.go — Revoke: non-super-admin
// =============================================================================

func TestAdminAPIKey_Revoke_NonSuperAdmin(t *testing.T) {
	h := NewAdminAPIKeyHandler(newMockStore(), newMockBoltStore())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/api-keys/pfx", nil)
	req.SetPathValue("prefix", "pfx")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// =============================================================================
// agent_status.go — List: container listing error
// =============================================================================

func TestAgentStatus_List_ContainerListError(t *testing.T) {
	c := testCoreWithBuild()
	mockRt := &mockContainerRuntime{listErr: fmt.Errorf("docker error")}
	c.Services.Container = mockRt
	handler := NewAgentStatusHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local, ok := resp["local"].(map[string]any)
	if !ok {
		t.Fatal("expected local agent info")
	}
	if local["containers"].(float64) != 0 {
		t.Errorf("expected 0 containers on error, got %v", local["containers"])
	}
}

func TestAgentStatus_List_NilContainerRuntime(t *testing.T) {
	c := testCoreWithBuild()
	handler := NewAgentStatusHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local, ok := resp["local"].(map[string]any)
	if !ok {
		t.Fatal("expected local agent info")
	}
	if local["server_id"] != "local" {
		t.Errorf("expected server_id=local, got %v", local["server_id"])
	}
}

// =============================================================================
// agent_status.go — GetAgent: missing path param, "local" redirect
// =============================================================================

func TestAgentStatus_GetAgent_MissingID(t *testing.T) {
	handler := NewAgentStatusHandler(testCoreWithBuild())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	handler.GetAgent(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAgentStatus_GetAgent_LocalRedirectToSelf(t *testing.T) {
	handler := NewAgentStatusHandler(testCoreWithBuild())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/local", nil)
	req.SetPathValue("id", "local")
	rr := httptest.NewRecorder()
	handler.GetAgent(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local, ok := resp["local"].(map[string]any)
	if !ok {
		t.Fatal("expected local agent info")
	}
	if local["server_id"] != "local" {
		t.Errorf("expected server_id=local, got %v", local["server_id"])
	}
}

// =============================================================================
// apps.go — Delete: store error paths
// =============================================================================

func TestAppDelete_AppDeleteError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	store.errDeleteApp = fmt.Errorf("delete error")
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to delete application")
}

func TestAppDelete_NoContainerRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore() // nil container runtime
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// apps.go — Restart: error paths
// =============================================================================

func TestAppRestart_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restart(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "container runtime not available")
}

func TestAppRestart_ContainerLookupError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{listErr: fmt.Errorf("docker error")}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "lookup container failed")
}

func TestAppRestart_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restart(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppRestart_RestartError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "cid123"}},
		restartErr: fmt.Errorf("container restart failed"),
	}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "restart failed")
}

// =============================================================================
// apps.go — Stop: error paths
// =============================================================================

func TestAppStop_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stop(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "container runtime not available")
}

func TestAppStop_ContainerLookupError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{listErr: fmt.Errorf("list error")}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stop(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppStop_NoContainerIdempotent(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stop(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "stopped" {
		t.Errorf("expected status stopped, got %q", resp["status"])
	}
	if resp["action"] != "noop" {
		t.Errorf("expected action noop, got %q", resp["action"])
	}
}

func TestAppStop_StopError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "cid123"}},
		stopErr:    fmt.Errorf("stop failed"),
	}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stop(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "stop failed")
}

// =============================================================================
// apps.go — Start: error paths
// =============================================================================

func TestAppStart_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "container runtime not available")
}

func TestAppStart_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppStart_ContainerLookupError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{listErr: fmt.Errorf("docker error")}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "lookup container failed")
}

func TestAppStart_RestartError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	c := testCore()
	mockRt := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "cid123"}},
		restartErr: fmt.Errorf("start/restart failed"),
	}
	c.Services.Container = mockRt
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "start failed")
}

// =============================================================================
// apps.go — findAppContainerID: nil runtime, list error, empty, success
// =============================================================================

func TestFindAppContainerID_NilRuntimeEdge(t *testing.T) {
	c := testCore()
	handler := NewAppHandler(newMockStore(), c)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id, err := handler.findAppContainerID(req, "app1")
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
	if !errors.Is(err, core.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestFindAppContainerID_ListError(t *testing.T) {
	c := testCore()
	mockRt := &mockContainerRuntime{listErr: fmt.Errorf("list error")}
	c.Services.Container = mockRt
	handler := NewAppHandler(newMockStore(), c)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id, err := handler.findAppContainerID(req, "app1")
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestFindAppContainerID_EmptyList(t *testing.T) {
	c := testCore()
	mockRt := &mockContainerRuntime{}
	c.Services.Container = mockRt
	handler := NewAppHandler(newMockStore(), c)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id, err := handler.findAppContainerID(req, "app1")
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestFindAppContainerID_Success(t *testing.T) {
	c := testCore()
	mockRt := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "cid123"}},
	}
	c.Services.Container = mockRt
	handler := NewAppHandler(newMockStore(), c)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id, err := handler.findAppContainerID(req, "app1")
	if id != "cid123" {
		t.Errorf("expected cid123, got %q", id)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// =============================================================================
// auth.go — NewAuthHandler: TOTP validator setup
// =============================================================================

func TestNewAuthHandler_WithTOTPService(t *testing.T) {
	store := newMockStore()
	totpSvc := internalAuth.NewTOTPService(nil)
	authMod := &testAuthServicesWithTOTP{jwt: testJWT(), totp: totpSvc}
	h := NewAuthHandler(authMod, store, newMockBoltStore())
	if h.totpValidator == nil {
		t.Error("expected totpValidator to be set")
	}
}

func TestNewAuthHandler_TOTPNotCalledWhenNil(t *testing.T) {
	store := newMockStore()
	authMod := &testAuthServices{jwt: testJWT()}
	h := NewAuthHandler(authMod, store, newMockBoltStore())
	if h.validateTOTP("user1", "123456") {
		t.Error("expected validateTOTP to return false when no TOTP configured")
	}
}

// =============================================================================
// auth.go — log(): nil logger falls back to default
// =============================================================================

func TestAuthLog_NilLogger(t *testing.T) {
	h := &AuthHandler{logger: nil}
	logger := h.log()
	if logger == nil {
		t.Error("log() should never return nil")
	}
}

func TestAuthLog_WithLogger(t *testing.T) {
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &AuthHandler{logger: l}
	logger := h.log()
	if logger != l {
		t.Error("log() should return the set logger")
	}
}

// =============================================================================
// auth.go — Login: long password, TOTP required but missing, invalid TOTP
// =============================================================================

func TestLogin_LongPassword(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	longPW := strings.Repeat("a", 257)
	body, _ := json.Marshal(loginRequest{Email: "test@test.com", Password: longPW})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "password must not exceed 256 characters")
}

func TestLogin_TOTPRequiredButMissing(t *testing.T) {
	store := newMockStore()
	user := seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	user.TOTPEnabled = true

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "TOTP code required")
	if rr.Header().Get("X-TOTP-Required") != "true" {
		t.Error("expected X-TOTP-Required header")
	}
}

func TestLogin_InvalidTOTPCode(t *testing.T) {
	store := newMockStore()
	user := seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	user.TOTPEnabled = true

	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "Password1", TOTPCode: "123456"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "invalid TOTP code")
}

// =============================================================================
// auth.go — Refresh: cookie fallback, revoked token, user not found
// =============================================================================

func TestRefresh_CookieFallback(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("user1", "tenant1", "role_owner", "user@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte("{}")))
	req.AddCookie(&http.Cookie{Name: cookieRefresh, Value: refreshToken})
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRefresh_NoToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "refresh_token required")
}

func TestRefresh_InvalidTokenEdge(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(refreshRequest{RefreshToken: "invalid-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "invalid refresh token")
}

func TestRefresh_RevokedToken(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	refreshToken := generateTestRefreshToken("user1", "tenant1", "role_owner", "user@example.com")

	rtClaims, err := authMod.JWT().ValidateRefreshToken(refreshToken)
	if err != nil {
		t.Fatalf("failed to validate refresh token: %v", err)
	}

	_ = bolt.Set("revoked_tokens", rtClaims.JTI, true, 3600)

	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "token has been revoked")
}

func TestRefresh_UserNotFoundEdge(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("nonexistent", "tenant1", "role_owner", "user@example.com")

	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "user not found")
}

// =============================================================================
// auth.go — Logout: cookie fallback, access token revocation via cookie (line 442-461)
// =============================================================================

func TestLogout_NoRefreshToken_OnlyAccessTokenRevoke(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	// Empty body, no cookie — should clear cookies and return OK
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestLogout_InvalidRefreshToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(logoutRequest{RefreshToken: "invalid-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestLogout_SuccessWithRevocation(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	refreshToken := generateTestRefreshToken("user1", "tenant1", "role_owner", "user@example.com")

	body, _ := json.Marshal(logoutRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// auth.go — revokeAccessTokenFromRequest: Authorization header + cookie path
// =============================================================================

func TestRevokeAccessTokenFromRequest_NoBolt(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	// Should return early without panic when bolt is nil
	handler.revokeAccessTokenFromRequest(req)
}

func TestRevokeAccessTokenFromRequest_NoToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	handler.revokeAccessTokenFromRequest(req)
	// No panic = success
}

func TestRevokeAccessTokenFromRequest_InvalidToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	handler.revokeAccessTokenFromRequest(req)
	// No panic = success (invalid token returns early)
}

func TestRevokeAccessTokenFromRequest_CookiePath(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieAccess, Value: "invalid-cookie-token"})
	handler.revokeAccessTokenFromRequest(req)
	// No panic = success (invalid cookie token returns early)
}

// =============================================================================
// auth.go — trackSession: errors paths (bolt nil, token parse failure, set error)
// =============================================================================

func TestTrackSession_NilBolt(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	// Should return early without panic
	handler.trackSession(httptest.NewRequest(http.MethodPost, "/", nil), "user1", "refresh-token")
}

func TestTrackSession_InvalidRefreshToken(t *testing.T) {
	store := newMockStore()
	authMod := testAuthModule(store)
	bolt := newMockBoltStore()
	handler := NewAuthHandler(authMod, store, bolt)

	// Invalid token — should return early
	handler.trackSession(httptest.NewRequest(http.MethodPost, "/", nil), "user1", "invalid-token")
}

// =============================================================================
// backups.go — List: unauthorized, storage error
// =============================================================================

func TestBackupList_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

// =============================================================================
// backups.go — Create: trigger error path
// =============================================================================

type errBackupTrigger struct{}

func (e *errBackupTrigger) TriggerNow(_ context.Context) error {
	return fmt.Errorf("trigger error")
}

func TestBackupCreate_TriggerError(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)
	handler.SetTrigger(&errBackupTrigger{})

	body, _ := json.Marshal(map[string]string{"source_type": "volume", "source_id": "vol-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to start backup")
}

// =============================================================================
// backups.go — Download: nil storage, download error
// =============================================================================

func TestBackupDownload_NilStorage(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/tenant1/test/restore", nil)
	req.SetPathValue("key", "tenant1/test")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Download(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "backup storage not configured")
}

func TestBackupDownload_NotFound(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{errDown: fmt.Errorf("not found")}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/tenant1/test/dl", nil)
	req.SetPathValue("key", "tenant1/test")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Download(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "backup not found")
}

func TestBackupDownload_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/tenant1/test/dl", nil)
	req.SetPathValue("key", "tenant1/test")
	rr := httptest.NewRecorder()

	handler.Download(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// =============================================================================
// certificates.go — List: unauthorized (no claims)
// =============================================================================

func TestCertificateList_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/certificates", nil)
	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

// =============================================================================
// certificates.go — Upload: no claims, bolt set errors
// =============================================================================

func TestCertificateUpload_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body, _ := json.Marshal(uploadCertRequest{
		DomainID: "domain1",
		CertPEM:  "cert",
		KeyPEM:   "key",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

// =============================================================================
// certificates.go — validateCertDomain: edge cases
// =============================================================================

func TestValidateCertDomain_EmptyDomain(t *testing.T) {
	err := validateCertDomain([]byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"), "")
	if err != nil {
		t.Errorf("expected nil for empty domain, got %v", err)
	}
}

func TestValidateCertDomain_InvalidPEM(t *testing.T) {
	err := validateCertDomain([]byte("not-pem-data"), "example.com")
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if err.Error() != "certificate is not valid PEM" {
		t.Errorf("expected 'certificate is not valid PEM', got %q", err.Error())
	}
}

// genSelfSignedCertWithSANs creates a self-signed cert with given DNS SANs and CN.
func genSelfSignedCertWithSANs(commonName string, dnsNames []string) []byte {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     dnsNames,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

func TestValidateCertDomain_NoDNSNames(t *testing.T) {
	certPEM := genSelfSignedCertWithSANs("", nil)
	err := validateCertDomain(certPEM, "example.com")
	if err == nil {
		t.Fatal("expected error for cert with no DNS names")
	}
	if !strings.Contains(err.Error(), "no DNS names") {
		t.Errorf("expected 'no DNS names' error, got %q", err.Error())
	}
}

func TestValidateCertDomain_IncorrectDomain(t *testing.T) {
	certPEM := genSelfSignedCertWithSANs("wrong.com", []string{"wrong.com"})
	err := validateCertDomain(certPEM, "example.com")
	if err == nil {
		t.Fatal("expected error for domain mismatch")
	}
	if !strings.Contains(err.Error(), "does not match domain") {
		t.Errorf("expected mismatch error, got %q", err.Error())
	}
}

func TestValidateCertDomain_CorrectDomainViaSAN(t *testing.T) {
	certPEM := genSelfSignedCertWithSANs("", []string{"example.com"})
	err := validateCertDomain(certPEM, "example.com")
	if err != nil {
		t.Errorf("expected nil for matching SAN, got %v", err)
	}
}

func TestValidateCertDomain_WildcardSANMatch(t *testing.T) {
	certPEM := genSelfSignedCertWithSANs("", []string{"*.example.com"})
	err := validateCertDomain(certPEM, "app.example.com")
	if err != nil {
		t.Errorf("expected nil for wildcard SAN match, got %v", err)
	}
}

// =============================================================================
// certificates.go — certMatchesDomain: CN fallback, wildcard CN
// =============================================================================

func TestCertMatchesDomain_CNFallback(t *testing.T) {
	// cert with only CN set, no SANs
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	if !certMatchesDomain(cert, "example.com") {
		t.Error("expected certMatchesDomain to match via CN")
	}
}

func TestCertMatchesDomain_WildcardCN(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "*.example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	if !certMatchesDomain(cert, "app.example.com") {
		t.Error("expected certMatchesDomain to match wildcard CN")
	}
}

func TestCertMatchesDomain_NoMatch(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"example.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	if certMatchesDomain(cert, "evil.com") {
		t.Error("expected certMatchesDomain to not match different domain")
	}
}

func TestCertMatchesDomain_WildcardSANSubdomain(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "root.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"*.example.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	if !certMatchesDomain(cert, "sub.example.com") {
		t.Error("expected wildcard *.example.com to match sub.example.com")
	}
	if certMatchesDomain(cert, "example.com") {
		t.Error("expected wildcard *.example.com to NOT match bare example.com")
	}
}

// =============================================================================
// certificates.go — Upload: domain not found in requireTenantCertificateDomain (line 179)
// =============================================================================

func TestCertificateUpload_DomainNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	// Generate a valid cert first since Upload validates the cert/key pair
	certPEM, keyPEM := testCertForDomain("example.com")

	body, _ := json.Marshal(uploadCertRequest{
		DomainID: "nonexistent-domain",
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(internalAuth.ContextWithClaims(req.Context(), &internalAuth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "domain not found")
}

// =============================================================================
// certificates.go — requireTenantCertificateDomain: store error, app error
// =============================================================================

func TestRequireTenantCertificateDomain_StoreError(t *testing.T) {
	store := newMockStore()
	store.errGetDomain = fmt.Errorf("db error")
	handler := NewCertificateHandler(store, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	domain, ok := handler.requireTenantCertificateDomain(rr, req, "domain1", "tenant1")
	if ok {
		t.Error("expected ok=false on store error")
	}
	if domain != nil {
		t.Errorf("expected nil domain, got %v", domain)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRequireTenantCertificateDomain_FQDNLookupError(t *testing.T) {
	store := newMockStore()
	store.errGetDomain = core.ErrNotFound
	store.errGetDomainByFQDN = fmt.Errorf("fqdn error")
	handler := NewCertificateHandler(store, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	domain, ok := handler.requireTenantCertificateDomain(rr, req, "example.com", "tenant1")
	if ok {
		t.Error("expected ok=false on FQDN lookup error")
	}
	if domain != nil {
		t.Errorf("expected nil domain, got %v", domain)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRequireTenantCertificateDomain_AppLookupError(t *testing.T) {
	store := newMockStore()
	store.addDomain(&core.Domain{ID: "domain1", AppID: "app1", FQDN: "example.com"})
	store.errGetApp = fmt.Errorf("app lookup error")
	handler := NewCertificateHandler(store, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	domain, ok := handler.requireTenantCertificateDomain(rr, req, "domain1", "tenant1")
	if ok {
		t.Error("expected ok=false on app lookup error")
	}
	if domain != nil {
		t.Errorf("expected nil domain, got %v", domain)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRequireTenantCertificateDomain_TenantMismatch(t *testing.T) {
	store := newMockStore()
	store.addDomain(&core.Domain{ID: "domain1", AppID: "app1", FQDN: "example.com"})
	store.addApp(&core.Application{ID: "app1", TenantID: "other-tenant"})
	handler := NewCertificateHandler(store, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	domain, ok := handler.requireTenantCertificateDomain(rr, req, "domain1", "tenant1")
	if ok {
		t.Error("expected ok=false on tenant mismatch")
	}
	if domain != nil {
		t.Errorf("expected nil domain, got %v", domain)
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireTenantCertificateDomain_Success(t *testing.T) {
	store := newMockStore()
	store.addDomain(&core.Domain{ID: "domain1", AppID: "app1", FQDN: "example.com"})
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	handler := NewCertificateHandler(store, newMockBoltStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	domain, ok := handler.requireTenantCertificateDomain(rr, req, "domain1", "tenant1")
	if !ok {
		t.Fatal("expected ok=true on success")
	}
	if domain == nil {
		t.Fatal("expected non-nil domain")
	}
	if domain.ID != "domain1" {
		t.Errorf("expected domain1, got %q", domain.ID)
	}
}

// =============================================================================
// certificates.go — Upload: bolt set errors
// =============================================================================

func TestCertificateUpload_BoltCertDataError(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	bolt.errSet = fmt.Errorf("bolt write error")
	handler := NewCertificateHandler(store, bolt)

	// domain not found — but first it tries to parse cert/key pair
	// We need a valid cert pair AND a domain in the store that matches
	certPEM, keyPEM := testCertForDomain("example.com")
	store.addApp(&core.Application{ID: "app1", TenantID: "test-tenant"})
	store.addDomain(&core.Domain{ID: "domain1", AppID: "app1", FQDN: "example.com"})

	body, _ := json.Marshal(uploadCertRequest{
		DomainID: "domain1",
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(internalAuth.ContextWithClaims(req.Context(), &internalAuth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to store certificate")
}

// =============================================================================
// backups.go — Restore: nil storage, download error, invalid format, create app error
// =============================================================================

func TestBackupRestore_NilStorage(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/tenant1/test/restore", nil)
	req.SetPathValue("key", "tenant1/test")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restore(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "backup storage not configured")
}

func TestBackupRestore_DownloadError(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{errDown: fmt.Errorf("download failed")}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/tenant1/test/restore", nil)
	req.SetPathValue("key", "tenant1/test")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restore(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "backup not found")
}

func TestBackupRestore_InvalidFormat(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{fileData: "not-json-at-all"}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/tenant1/test/restore", nil)
	req.SetPathValue("key", "tenant1/test")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restore(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "invalid backup format")
}

func TestBackupRestore_CreateAppError(t *testing.T) {
	store := newMockStore()
	store.errCreateApp = fmt.Errorf("create failed")
	appJSON := `{"id":"app-1","tenant_id":"tenant1","name":"my-app","type":"docker","status":"running","replicas":1}`
	storage := &mockBackupStorage{fileData: appJSON}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/tenant1/test/restore", nil)
	req.SetPathValue("key", "tenant1/test")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restore(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to restore app")
}

// =============================================================================
// apps.go — Create: field validation error for long fields
// =============================================================================

func TestCreateApp_FieldValidation_LongFields(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:       "my-app",
		SourceURL:  "https://x.com/" + strings.Repeat("a", 3000),
		Branch:     strings.Repeat("b", 200),
		Type:       strings.Repeat("c", 100),
		SourceType: strings.Repeat("d", 100),
		ProjectID:  strings.Repeat("e", 200),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// apps.go — Create: conflicting name
// =============================================================================

func TestCreateApp_DuplicateName(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "existing", TenantID: "tenant1", Name: "my-app"})
	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{Name: "my-app"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "application with this name already exists")
}

// =============================================================================
// admin.go — ListTenants: tenants list error
// =============================================================================

func TestAdminListTenants_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListAllTenants = fmt.Errorf("db error")
	handler := NewAdminHandler(testCoreWithBuild(), store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants", nil)
	rr := httptest.NewRecorder()

	handler.ListTenants(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to list tenants")
}

// =============================================================================
// admin.go — RevokeAllKeys: nil authMod (line 102-105)
// =============================================================================

func TestAdminHandler_RevokeAllKeys_NilAuthMod(t *testing.T) {
	handler := NewAdminHandler(testCoreWithBuild(), newMockStore())
	// authMod is nil

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/revoke-all", nil)
	rr := httptest.NewRecorder()

	handler.RevokeAllKeys(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "auth service unavailable")
}

// =============================================================================
// auth.go — enforceSessionLimit: nil bolt returns early
// =============================================================================

func TestEnforceSessionLimit_NilBolt(t *testing.T) {
	handler := &AuthHandler{}
	// Should not panic
	handler.enforceSessionLimit("user1")
}

// =============================================================================
// auth.go — checkPerAccountRateLimit: nil bolt returns early
// =============================================================================

func TestCheckPerAccountRateLimit_NilBolt(t *testing.T) {
	handler := &AuthHandler{}
	locked, until := handler.checkPerAccountRateLimit("user@test.com")
	if locked {
		t.Error("expected not locked when bolt is nil")
	}
	if until != 0 {
		t.Errorf("expected until=0, got %d", until)
	}
}

// =============================================================================
// auth.go — incrementPerAccountRateLimit: nil bolt returns early
// =============================================================================

func TestIncrementPerAccountRateLimit_NilBolt(t *testing.T) {
	handler := &AuthHandler{}
	handler.incrementPerAccountRateLimit(context.Background(), "user@test.com")
	// No panic = success
}

// =============================================================================
// auth.go — loginRateLimitCheck: nil bolt returns early
// =============================================================================

func TestLoginRateLimitCheck_NilBoltEdge(t *testing.T) {
	handler := &AuthHandler{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	result := handler.loginRateLimitCheck(rr, req, "user@test.com")
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

// =============================================================================
// auth.go — Login: GetUserMembership error (line 243-247)
// =============================================================================

func TestLogin_GetUserMembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	store.errGetUserMembership = fmt.Errorf("membership error")
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	body, _ := json.Marshal(loginRequest{Email: "user@example.com", Password: "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// auth.go — Refresh: GetUserMembership error (line 396-399)
// =============================================================================

func TestRefresh_GetUserMembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "user1", "user@example.com", "Password1", "tenant1", "role_owner")
	store.errGetUserMembership = fmt.Errorf("membership error")
	authMod := testAuthModule(store)
	handler := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("user1", "tenant1", "role_owner", "user@example.com")

	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Refresh(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "internal error")
}

