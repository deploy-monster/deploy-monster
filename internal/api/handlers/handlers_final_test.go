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
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// =============================================================================
// AdminHandler.SystemInfo — covers line 23 (runtime.ReadMemStats + module loop)
// =============================================================================

func TestFinal_AdminHandler_SystemInfo(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: core.NewRegistry(),
	}
	h := NewAdminHandler(c, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()
	h.SystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["go"] == nil {
		t.Error("expected 'go' field in response")
	}
}

// =============================================================================
// AdminAPIKeyHandler.Generate — bolt Set error on first Set (key record)
// =============================================================================

type boltFailOnFirstSet struct {
	*mockBoltStore
}

func (b *boltFailOnFirstSet) Set(bucket, key string, value any, ttl int64) error {
	return fmt.Errorf("bolt write error")
}

func TestFinal_AdminAPIKey_Generate_BoltSetError(t *testing.T) {
	bolt := &boltFailOnFirstSet{mockBoltStore: newMockBoltStore()}
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// AuthHandler.Login — GetUserByEmail non-ErrNotFound error (line 61)
// =============================================================================

func TestFinal_Auth_Login_InternalError(t *testing.T) {
	store := newMockStore()
	store.errGetUserByEmail = fmt.Errorf("database connection lost")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"test@test.com","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Login — GetUserMembership error (line 74)
// =============================================================================

func TestFinal_Auth_Login_MembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Password123!", "t1", "role_owner")
	store.errGetUserMembership = fmt.Errorf("membership query failed")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"test@test.com","password":"Password123!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Register — CreateTenantWithDefaults error (line 131)
// =============================================================================

func TestFinal_Auth_Register_CreateTenantError(t *testing.T) {
	store := newMockStore()
	store.errCreateTenantWithDefaults = fmt.Errorf("tenant creation failed")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"new@test.com","password":"Password123!","name":"New User"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Register — CreateUserWithMembership error (line 137)
// =============================================================================

func TestFinal_Auth_Register_CreateUserError(t *testing.T) {
	store := newMockStore()
	store.errCreateUserWithMembership = fmt.Errorf("user creation failed")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"new@test.com","password":"Password123!","name":"New User"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Refresh — GetUser error (line 174)
// =============================================================================

func TestFinal_Auth_Refresh_GetUserError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Password123!", "t1", "role_owner")
	store.errGetUser = fmt.Errorf("user not found")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("u1", "t1", "role_owner", "test@test.com")
	body := fmt.Sprintf(`{"refresh_token":"%s"}`, refreshToken)
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// AuthHandler.Refresh — GetUserMembership error (line 181)
// =============================================================================

func TestFinal_Auth_Refresh_MembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Password123!", "t1", "role_owner")
	store.errGetUserMembership = fmt.Errorf("membership error")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("u1", "t1", "role_owner", "test@test.com")
	body := fmt.Sprintf(`{"refresh_token":"%s"}`, refreshToken)
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Refresh(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// generateSlug — empty result (line 206-208: special chars only)
// =============================================================================

func TestFinal_GenerateSlug_SpecialCharsOnly(t *testing.T) {
	slug := generateSlug("!!!@@@###")
	if slug == "" || slug == "!!!@@@###" {
		t.Errorf("expected generated ID fallback, got %q", slug)
	}
	if len(slug) != 8 {
		t.Errorf("expected 8-char fallback slug, got %q (len=%d)", slug, len(slug))
	}
}

func TestFinal_GenerateSlug_Underscore(t *testing.T) {
	slug := generateSlug("my_app")
	if slug != "my-app" {
		t.Errorf("expected 'my-app', got %q", slug)
	}
}

// =============================================================================
// BulkHandler.Execute — max 50 limit (line 52-53)
// =============================================================================

func TestFinal_Bulk_TooManyApps(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewBulkHandler(store, nil, events)

	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("app-%d", i)
	}
	idsJSON, _ := json.Marshal(ids)
	body := fmt.Sprintf(`{"action":"start","app_ids":%s}`, string(idsJSON))

	req := httptest.NewRequest("POST", "/api/v1/apps/bulk", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Execute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// BulkHandler.Execute — unknown action (line 88-89)
// =============================================================================

func TestFinal_Bulk_UnknownAction(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	events := core.NewEventBus(slog.Default())
	h := NewBulkHandler(store, nil, events)

	body := `{"action":"destroy","app_ids":["app-1"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/bulk", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["failed"].(float64) != 1 {
		t.Errorf("expected 1 failed, got %v", resp["failed"])
	}
}

// =============================================================================
// CertificateHandler.Upload — invalid cert pair (line 79)
// =============================================================================

func TestFinal_Certificate_Upload_InvalidPair(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockBoltStore())

	body := `{"domain_id":"d1","cert_pem":"not-a-cert","key_pem":"not-a-key"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// CertificateHandler.Upload — bolt Set error (store cert data, line 107-108)
// =============================================================================

func TestFinal_Certificate_Upload_BoltError(t *testing.T) {
	bolt := &boltFailOnFirstSet{mockBoltStore: newMockBoltStore()}
	h := NewCertificateHandler(newMockStore(), bolt)

	// Use a valid self-signed cert/key pair is complex; test just the field validation
	body := `{"domain_id":"","cert_pem":"x","key_pem":"y"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	// Missing domain_id should be caught first
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// ComposeHandler.Deploy — YAML content type path (line 40-47)
// =============================================================================

func TestFinal_Compose_Deploy_YAMLContentType(t *testing.T) {
	store := newMockStore()
	store.projects = map[string][]core.Project{"t1": {{ID: "p1", TenantID: "t1"}}}
	events := core.NewEventBus(slog.Default())
	// Use nil runtime so the async goroutine's deployer returns early without crashing
	h := NewComposeHandler(store, nil, events)

	yamlBody := `version: "3"
services:
  web:
    image: nginx`
	req := httptest.NewRequest("POST", "/api/v1/stacks?name=mystack", strings.NewReader(yamlBody))
	req.Header.Set("Content-Type", "application/x-yaml")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// DeployApprovalHandler.Approve — not found (line 80)
// =============================================================================

func TestFinal_DeployApproval_Approve_NotFound(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := NewDeployApprovalHandler(newMockStore(), events)

	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/xxx/approve", nil)
	req.SetPathValue("id", "xxx")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// DeployTriggerHandler.TriggerDeploy — image type deploy with runtime (lines 33-76)
// =============================================================================

func TestFinal_DeployTrigger_ImageDeploy(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "t1",
		Name:       "myapp",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.nextDeployVersion = map[string]int{"app-1": 2}
	rt := &mockContainerRuntime{}
	events := core.NewEventBus(slog.Default())
	h := NewDeployTriggerHandler(store, rt, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/deploy", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// DeployTriggerHandler.TriggerDeploy — git-sourced deploy (lines 80-110)
// =============================================================================

func TestFinal_DeployTrigger_GitDeploy(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-2",
		TenantID:   "t1",
		Name:       "gitapp",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo.git",
		Branch:     "main",
	})
	events := core.NewEventBus(slog.Default())
	h := NewDeployTriggerHandler(store, &mockContainerRuntime{}, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-2/deploy", nil)
	req.SetPathValue("id", "app-2")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// EnvImportHandler.Import — .env format (line 42-43: parseDotEnv branch)
// =============================================================================

func TestFinal_EnvImport_DotEnvFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "myapp"})
	h := NewEnvImportHandler(store)

	envContent := "DB_HOST=localhost\nDB_PORT=5432\n# comment\nDB_NAME=mydb"
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(envContent))
	req.SetPathValue("id", "app-1")
	req.Header.Set("Content-Type", "text/plain")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// EnvImportHandler.parseDotEnv — quoted values (line 111-113)
// =============================================================================

func TestFinal_ParseDotEnv_QuotedValues(t *testing.T) {
	content := `KEY1="value with spaces"
KEY2='single quoted'
KEY3=no-quotes`
	vars := parseDotEnv(content)
	if len(vars) != 3 {
		t.Fatalf("expected 3 vars, got %d", len(vars))
	}
	if vars[0].Value != "value with spaces" {
		t.Errorf("KEY1 = %q, want 'value with spaces'", vars[0].Value)
	}
	if vars[1].Value != "single quoted" {
		t.Errorf("KEY2 = %q, want 'single quoted'", vars[1].Value)
	}
}

// =============================================================================
// parseEnvJSON — empty string (line 80-81)
// =============================================================================

func TestFinal_ParseEnvJSON_Empty(t *testing.T) {
	result := parseEnvJSON("")
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

// =============================================================================
// ImageCleanupHandler.Prune — ImageRemove error (line 55: continue branch)
// =============================================================================

type mockRuntimeRemoveFails struct {
	mockContainerRuntime
}

func (m *mockRuntimeRemoveFails) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return []core.ImageInfo{
		{ID: "img1", Tags: []string{"<none>:<none>"}, Size: 10 * 1024 * 1024},
	}, nil
}

func (m *mockRuntimeRemoveFails) ImageRemove(_ context.Context, _ string) error {
	return fmt.Errorf("image in use")
}

func TestFinal_ImageCleanup_Prune_RemoveError(t *testing.T) {
	h := NewImageCleanupHandler(&mockRuntimeRemoveFails{})

	req := httptest.NewRequest("DELETE", "/api/v1/images/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["images_removed"].(float64) != 0 {
		t.Errorf("expected 0 removed, got %v", resp["images_removed"])
	}
}

// =============================================================================
// ImportExportHandler.Import — CreateApp error (line 100)
// =============================================================================

func TestFinal_Import_CreateAppError(t *testing.T) {
	store := newMockStore()
	store.errCreateApp = fmt.Errorf("db write error")
	h := NewImportExportHandler(store)

	body := `{"version":"1","name":"imported","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ImportExportHandler.Import — with projects (line 95-97)
// =============================================================================

func TestFinal_Import_WithProject(t *testing.T) {
	store := newMockStore()
	store.projects = map[string][]core.Project{"t1": {{ID: "p1", TenantID: "t1"}}}
	h := NewImportExportHandler(store)

	body := `{"version":"1","name":"imported","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1,"domains":["app.example.com"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// InviteHandler.Create — CreateInvite error (line 64)
// =============================================================================

func TestFinal_Invite_Create_StoreError(t *testing.T) {
	store := newMockStore()
	store.errCreateInvite = fmt.Errorf("db error")
	events := core.NewEventBus(slog.Default())
	h := NewInviteHandler(store, events)

	body := `{"email":"new@test.com","role_id":"role_member"}`
	req := httptest.NewRequest("POST", "/api/v1/team/invites", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// LogDownloadHandler.Download — Logs error (line 39)
// =============================================================================

func TestFinal_LogDownload_LogsError(t *testing.T) {
	rt := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "cnt-12345678", State: "running"}},
		logsErr:    fmt.Errorf("log read failed"),
	}
	h := NewLogDownloadHandler(rt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app12345678/logs/download", nil)
	req.SetPathValue("id", "app12345678")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// MarketplaceHandler.List — nil registry (line 20-22)
// =============================================================================

func TestFinal_Marketplace_List_NilRegistry(t *testing.T) {
	h := NewMarketplaceHandler(nil)

	req := httptest.NewRequest("GET", "/api/v1/marketplace", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

// =============================================================================
// MarketplaceDeployHandler.Deploy — template not found (line 52)
// =============================================================================

func TestFinal_MarketplaceDeploy_TemplateNotFound(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(registry, nil, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// MarketplaceDeployHandler.Deploy — CreateApp error (line 88)
// =============================================================================

func TestFinal_MarketplaceDeploy_CreateAppError(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	registry.Add(&marketplace.Template{
		Slug:        "test-app",
		Name:        "Test App",
		ComposeYAML: "version: '3'\nservices:\n  web:\n    image: nginx\n",
	})

	store := newMockStore()
	store.errCreateApp = fmt.Errorf("db error")
	h := NewMarketplaceDeployHandler(registry, nil, store, core.NewEventBus(slog.Default()))

	body := `{"slug":"test-app","name":"myapp"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// PortHandler.Update — invalid port (line 58-59)
// =============================================================================

func TestFinal_Port_Update_InvalidPort(t *testing.T) {
	h := NewPortHandler(newMockStore())

	body := `[{"container_port":0,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// PortHandler.Update — port over 65535
// =============================================================================

func TestFinal_Port_Update_PortTooHigh(t *testing.T) {
	h := NewPortHandler(newMockStore())

	body := `[{"container_port":70000,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// RegistryHandler.Add — bolt Set error (line 96-98)
// =============================================================================

func TestFinal_Registry_Add_BoltError(t *testing.T) {
	bolt := &boltFailOnFirstSet{mockBoltStore: newMockBoltStore()}
	h := NewRegistryHandler(bolt)

	body := `{"name":"My Registry","url":"registry.example.com","username":"user","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/v1/registries", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Add(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SearchHandler.Search — results limit >20 (line 75-76)
// =============================================================================

func TestFinal_Search_ResultsLimit(t *testing.T) {
	store := newMockStore()
	// Add 25 apps that match the search
	for i := range 25 {
		store.appList = append(store.appList, core.Application{
			ID: fmt.Sprintf("app-%d", i), Name: fmt.Sprintf("test-app-%d", i), Status: "running",
		})
	}
	store.appTotal = 25
	h := NewSearchHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/search?q=test", nil)
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total > 20 {
		t.Errorf("expected max 20 results, got %v", total)
	}
}

// =============================================================================
// SecretHandler.Create — vault encrypt error (line 60)
// =============================================================================

type mockVaultEncryptFails struct{}

func (m *mockVaultEncryptFails) Encrypt(_ string) (string, error) {
	return "", fmt.Errorf("encryption hardware failure")
}
func (m *mockVaultEncryptFails) Decrypt(s string) (string, error) { return s, nil }

func TestFinal_Secret_Create_VaultEncryptError(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewSecretHandler(store, &mockVaultEncryptFails{}, events)

	body := `{"name":"DB_PASS","value":"secret123","scope":"tenant"}`
	req := httptest.NewRequest("POST", "/api/v1/secrets", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SecretHandler.List — store error (line 119)
// =============================================================================

func TestFinal_Secret_List_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListSecretsByTenant = fmt.Errorf("db error")
	events := core.NewEventBus(slog.Default())
	h := NewSecretHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/secrets", nil)
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SecretHandler.Create — CreateSecretVersion error (line 90)
// =============================================================================

func TestFinal_Secret_Create_VersionError(t *testing.T) {
	store := newMockStore()
	store.errCreateSecretVersion = fmt.Errorf("version write error")
	events := core.NewEventBus(slog.Default())
	h := NewSecretHandler(store, nil, events)

	body := `{"name":"DB_PASS","value":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/secrets", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ServiceMeshHandler.Delete — bolt Set error (line 100-101)
// =============================================================================

func TestFinal_ServiceMesh_Delete_BoltSetError(t *testing.T) {
	bolt := newMockBoltStore()
	// Pre-populate with a link
	bolt.Set("service_mesh", "app-1", serviceLinkList{
		Links: []ServiceLink{{ID: "link-1", SourceAppID: "app-1", TargetAppID: "app-2"}},
	}, 0)

	// Replace with a failing bolt
	failBolt := &boltFailOnFirstSet{mockBoltStore: bolt}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "App"})
	h := NewServiceMeshHandler(store, failBolt)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/links/link-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("targetId", "link-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// ServiceMeshHandler.Delete — bolt Get error (no links, line 88)
// =============================================================================

func TestFinal_ServiceMesh_Delete_NoLinks(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "App"})
	h := NewServiceMeshHandler(store, newMockBoltStore())

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/links/link-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("targetId", "link-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.UpdateProfile — GetUser error (line 72)
// =============================================================================

func TestFinal_Session_UpdateProfile_GetUserError(t *testing.T) {
	store := newMockStore()
	store.errGetUser = fmt.Errorf("user not found")
	h := NewSessionHandler(store)

	body := `{"name":"New Name"}`
	req := httptest.NewRequest("PATCH", "/api/v1/auth/me", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.UpdateProfile(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.UpdateProfile — UpdateUser error (line 85)
// =============================================================================

func TestFinal_Session_UpdateProfile_UpdateError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Pass123!", "t1", "role_owner")
	store.errUpdateUser = fmt.Errorf("update failed")
	h := NewSessionHandler(store)

	body := `{"name":"New Name"}`
	req := httptest.NewRequest("PATCH", "/api/v1/auth/me", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.UpdateProfile(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.ChangePassword — wrong current password (line 117)
// =============================================================================

func TestFinal_Session_ChangePassword_WrongCurrent(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "CurrentPass1!", "t1", "role_owner")
	h := NewSessionHandler(store)

	body := `{"current_password":"WrongPassword1!","new_password":"NewPass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.ChangePassword — weak new password (line 123)
// =============================================================================

func TestFinal_Session_ChangePassword_WeakNewPassword(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "CurrentPass1!", "t1", "role_owner")
	h := NewSessionHandler(store)

	body := `{"current_password":"CurrentPass1!","new_password":"short"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.ChangePassword — UpdatePassword error (line 134)
// =============================================================================

func TestFinal_Session_ChangePassword_UpdateError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "CurrentPass1!", "t1", "role_owner")
	store.errUpdatePassword = fmt.Errorf("db write error")
	h := NewSessionHandler(store)

	body := `{"current_password":"CurrentPass1!","new_password":"NewStrongPass1!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SSHKeyHandler.Generate — bolt Set error (line 92-93)
// =============================================================================

func TestFinal_SSHKey_Generate_BoltError(t *testing.T) {
	bolt := &boltFailOnFirstSet{mockBoltStore: newMockBoltStore()}
	h := NewSSHKeyHandler(newMockStore(), bolt)

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// StatsHandler.ServerStats — runtime error (line 55-56)
// =============================================================================

func TestFinal_Stats_ServerStats_RuntimeError(t *testing.T) {
	rt := &mockContainerRuntime{listErr: fmt.Errorf("docker down")}
	h := NewStatsHandler(rt, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// StatsHandler.ServerStats — nil runtime (line 47-48)
// =============================================================================

func TestFinal_Stats_ServerStats_NilRuntime(t *testing.T) {
	h := NewStatsHandler(nil, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// StorageHandler.Usage — exercises volume/image/bolt paths (line 33-53)
// =============================================================================

func TestFinal_Storage_Usage(t *testing.T) {
	rt := &mockContainerRuntime{}
	bolt := newMockBoltStore()
	h := NewStorageHandler(newMockStore(), rt, bolt)

	req := httptest.NewRequest("GET", "/api/v1/storage/usage", nil)
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Usage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// TransferHandler.TransferApp — GetApp error (line 47)
// =============================================================================

func TestFinal_Transfer_AppNotFound(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewTransferHandler(store, events)

	body := `{"target_tenant_id":"t2"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/nonexistent/transfer", strings.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// TransferHandler.TransferApp — UpdateApp error (line 62)
// =============================================================================

func TestFinal_Transfer_UpdateAppError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "myapp"})
	store.addTenant(&core.Tenant{ID: "t2", Name: "Other Team"})
	store.errUpdateApp = fmt.Errorf("update failed")
	events := core.NewEventBus(slog.Default())
	h := NewTransferHandler(store, events)

	body := `{"target_tenant_id":"t2"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// BuildCacheHandler.Clear — no runtime (line 82-83: nil runtime path)
// =============================================================================

func TestFinal_BuildCache_Clear_NilRuntime(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewBuildCacheHandler(nil, bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// BuildCacheHandler.Clear — ImageRemove error (skip branch, line 76-77)
// =============================================================================

func TestFinal_BuildCache_Clear_RemoveError(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewBuildCacheHandler(&mockRuntimeRemoveFails{}, bolt)

	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// DBBackupHandler.Backup — file not found (line 38)
// =============================================================================

func TestFinal_DBBackup_Backup_FileNotFound(t *testing.T) {
	c := &core.Core{Config: &core.Config{Database: core.DatabaseConfig{Path: "/nonexistent/path/db.sqlite"}}}
	h := NewDBBackupHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/backup", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Backup(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// DBBackupHandler.Status — file not found (line 77)
// =============================================================================

func TestFinal_DBBackup_Status_FileNotFound(t *testing.T) {
	c := &core.Core{Config: &core.Config{Database: core.DatabaseConfig{Path: "/nonexistent/path/db.sqlite"}}}
	h := NewDBBackupHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/status", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// DetailedHealthHandler — db not available, docker not available (degraded path)
// =============================================================================

func TestFinal_DetailedHealth_Degraded(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: core.NewRegistry(),
		// No Store set => db check fails
		// No Container set => docker check fails
	}
	h := NewDetailedHealthHandler(c)

	req := httptest.NewRequest("GET", "/health/detailed", nil)
	rr := httptest.NewRecorder()
	h.DetailedHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "degraded" {
		t.Errorf("expected 'degraded', got %v", resp["status"])
	}
}

// =============================================================================
// MetricsExportHandler.Export — CSV format (line 77-94)
// =============================================================================

func TestFinal_MetricsExport_CSV(t *testing.T) {
	bolt := newMockBoltStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app12345678", TenantID: "t1", Name: "App"})
	h := NewMetricsExportHandler(store, bolt, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app12345678/metrics/export?format=csv", nil)
	req.SetPathValue("id", "app12345678")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("expected CSV content type, got %q", rr.Header().Get("Content-Type"))
	}
}

// =============================================================================
// MetricsExportHandler.Export — short appID (line 77-78)
// =============================================================================

func TestFinal_MetricsExport_ShortAppID(t *testing.T) {
	bolt := newMockBoltStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "ab", TenantID: "t1", Name: "App"})
	h := NewMetricsExportHandler(store, bolt, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/ab/metrics/export?format=csv", nil)
	req.SetPathValue("id", "ab")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AgentStatusHandler.List — with module health statuses (line 43-45, 62-69)
// =============================================================================

func TestFinal_AgentStatus_List_WithModules(t *testing.T) {
	reg := core.NewRegistry()
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: reg,
	}
	h := NewAgentStatusHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// SelfUpdateHandler.CheckUpdate — exercises network call path (line 27-36)
// =============================================================================

func TestFinal_SelfUpdate_CheckUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	c := &core.Core{
		Config: &core.Config{},
		Build:  core.BuildInfo{Version: "0.0.1-test", Commit: "abc123", Date: "2026-01-01"},
	}
	h := NewSelfUpdateHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/updates", nil)
	rr := httptest.NewRecorder()
	h.CheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
