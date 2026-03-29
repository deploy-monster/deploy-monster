package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// DeployTriggerHandler — image app with runtime error
// =============================================================================

func TestDeployTrigger_ImageApp_RuntimeError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-err",
		TenantID:   "t1",
		Name:       "err-app",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})

	runtime := &failRuntime{err: fmt.Errorf("container start failed")}
	handler := NewDeployTriggerHandler(store, runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-err/deploy", nil)
	req.SetPathValue("id", "app-err")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "deploy failed: container start failed")

	if store.updatedStatus["app-err"] != "failed" {
		t.Errorf("expected status=failed, got %q", store.updatedStatus["app-err"])
	}
}

// failRuntime is a ContainerRuntime that returns error on CreateAndStart.
type failRuntime struct {
	mockContainerRuntime
	err error
}

func (f *failRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", f.err
}

// =============================================================================
// DNSRecordHandler — List with provider (verify error/success/missing domain)
// =============================================================================

func TestDNSRecordHandler_List_VerifyError(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{verifyErr: fmt.Errorf("DNS lookup timeout")})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=fail.com&provider=cloudflare", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_List_VerifySuccess(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{verified: true})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=ok.com&provider=cloudflare", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["verified"] != true {
		t.Errorf("expected verified=true, got %v", resp["verified"])
	}
}

// =============================================================================
// DNSRecordHandler — Create success / provider error
// =============================================================================

func TestDNSRecordHandler_Create_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{})
	h := NewDNSRecordHandler(services)

	body := `{"name":"test.example.com","value":"1.2.3.4","type":"A"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records?provider=cloudflare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDNSRecordHandler_Create_ProviderError(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{createErr: fmt.Errorf("rate limited")})
	h := NewDNSRecordHandler(services)

	body := `{"name":"test.example.com","value":"1.2.3.4","type":"A"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records?provider=cloudflare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// DNSRecordHandler — Delete success / error
// =============================================================================

func TestDNSRecordHandler_Delete_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("DELETE", "/api/v1/dns/records/rec-1?provider=cloudflare&name=example.com", nil)
	req.SetPathValue("id", "rec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Delete_Error(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{deleteErr: fmt.Errorf("not found")})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("DELETE", "/api/v1/dns/records/rec-1?provider=cloudflare&name=example.com", nil)
	req.SetPathValue("id", "rec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AdminAPIKeyHandler — Generate then List / bolt error
// =============================================================================

func TestAdminAPIKeys_GenerateThenList(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	handler := NewAdminAPIKeyHandler(store, bolt)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Generate: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys", nil)
	rr2 := httptest.NewRecorder()
	handler.List(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("List: expected 200, got %d", rr2.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr2.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 key, got %d", len(data))
	}
}

func TestAdminAPIKeys_Generate_BoltSetError(t *testing.T) {
	store := newMockStore()
	bolt := &errOnSetBolt{}
	handler := NewAdminAPIKeyHandler(store, bolt)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

type errOnSetBolt struct{ mockBoltStore }

func (e *errOnSetBolt) Set(_, _ string, _ any, _ int64) error {
	return fmt.Errorf("bolt write error")
}
func (e *errOnSetBolt) Get(_, _ string, _ any) error {
	return fmt.Errorf("key not found")
}

// =============================================================================
// OpenAPIHandler — Spec endpoint
// =============================================================================

func TestOpenAPIHandler_Spec(t *testing.T) {
	h := NewOpenAPIHandler("1.2.3")

	req := httptest.NewRequest("GET", "/api/v1/openapi.json", nil)
	rr := httptest.NewRecorder()
	h.Spec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", resp["openapi"])
	}
	info := resp["info"].(map[string]any)
	if info["version"] != "1.2.3" {
		t.Errorf("version = %v, want 1.2.3", info["version"])
	}
	paths, ok := resp["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Error("expected non-empty paths")
	}
}

// =============================================================================
// MigrationHandler — no claims / no DB
// =============================================================================

func TestMigrationHandler_NoClaims(t *testing.T) {
	c := testCore()
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestMigrationHandler_NoDB(t *testing.T) {
	c := testCore()
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// PlatformStatsHandler — no claims / success
// =============================================================================

func TestPlatformStatsHandler_NoClaims(t *testing.T) {
	c := testCore()
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestPlatformStatsHandler_Success(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	platform := resp["platform"].(map[string]any)
	if platform["version"] != "1.0.0" {
		t.Errorf("version = %v", platform["version"])
	}
}

// =============================================================================
// SSHKeyHandler — Generate / List
// =============================================================================

func TestSSHKeyHandler_Generate_Success(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSSHKeyHandler(newMockStore(), bolt)

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["name"] != "my-key" {
		t.Errorf("name = %v", resp["name"])
	}
	if resp["private_key"] == nil || resp["private_key"] == "" {
		t.Error("expected private_key in response")
	}
	if resp["public_key"] == nil || resp["public_key"] == "" {
		t.Error("expected public_key in response")
	}
	if resp["fingerprint"] == nil || resp["fingerprint"] == "" {
		t.Error("expected fingerprint in response")
	}
}

func TestSSHKeyHandler_Generate_NoClaims(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockBoltStore())

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSSHKeyHandler_Generate_MissingName(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockBoltStore())

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSHKeyHandler_List_NoClaims(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/ssh-keys", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSSHKeyHandler_List_Empty(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/ssh-keys", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// SSLStatusHandler — Check
// =============================================================================

func TestSSLStatusHandler_Check_MissingFQDN(t *testing.T) {
	h := NewSSLStatusHandler(newMockBoltStore())

	req := httptest.NewRequest("GET", "/api/v1/domains/d1/ssl-status", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSLStatusHandler_Check_Uncached(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSSLStatusHandler(bolt)

	// Use an unreachable host to get the error path of checkSSL
	req := httptest.NewRequest("GET", "/api/v1/domains/d1/ssl-status?fqdn=localhost.invalid.nxdomain", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp SSLCheckResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "localhost.invalid.nxdomain" {
		t.Errorf("fqdn = %q", resp.FQDN)
	}
	// Should have error since host is not reachable
	if resp.Error == "" {
		t.Error("expected error for unreachable host")
	}
}

// =============================================================================
// SSHTestHandler — Test endpoint
// =============================================================================

func TestSSHTestHandler_MissingHost(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":""}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSHTestHandler_InvalidBody(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader("bad json"))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSHTestHandler_UnreachableHost(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":"192.0.2.1","port":22}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != false {
		t.Errorf("expected reachable=false for unreachable host")
	}
}

// =============================================================================
// MarketplaceDeployHandler — constructor
// =============================================================================

func TestMarketplaceDeployHandler_New(t *testing.T) {
	h := NewMarketplaceDeployHandler(nil, nil, newMockStore(), testCore().Events)
	if h == nil {
		t.Fatal("NewMarketplaceDeployHandler returned nil")
	}
}

// =============================================================================
// SelfUpdateHandler — CheckUpdate
// =============================================================================

func TestSelfUpdateHandler_CheckUpdate(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "dev", Commit: "abc123", Date: "2026-01-01"},
		Registry: core.NewRegistry(),
	}
	h := NewSelfUpdateHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/updates", nil)
	rr := httptest.NewRecorder()
	h.CheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["current_version"] != "dev" {
		t.Errorf("current_version = %v", resp["current_version"])
	}
}

// =============================================================================
// Mock DNS provider
// =============================================================================

type mockDNS struct {
	verified  bool
	verifyErr error
	createErr error
	deleteErr error
}

func (m *mockDNS) Name() string                                           { return "mock" }
func (m *mockDNS) CreateRecord(_ context.Context, _ core.DNSRecord) error { return m.createErr }
func (m *mockDNS) UpdateRecord(_ context.Context, _ core.DNSRecord) error { return nil }
func (m *mockDNS) DeleteRecord(_ context.Context, _ core.DNSRecord) error { return m.deleteErr }
func (m *mockDNS) Verify(_ context.Context, _ string) (bool, error) {
	return m.verified, m.verifyErr
}
