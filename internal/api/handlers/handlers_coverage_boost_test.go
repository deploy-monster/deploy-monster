package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"crypto/x509"
	"crypto/x509/pkix"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── deploy_trigger.go coverage ──────────────────────────────────────────────

func TestDeployTriggerHandler_SetBuildImageRegistry(t *testing.T) {
	h := &DeployTriggerHandler{}

	// Normal case
	h.SetBuildImageRegistry(" registry.example.com/team/ ")
	if h.buildRepo != "registry.example.com/team" {
		t.Errorf("SetBuildImageRegistry with spaces = %q, want registry.example.com/team", h.buildRepo)
	}

	// Empty prefix
	h.SetBuildImageRegistry("")
	if h.buildRepo != "" {
		t.Errorf("SetBuildImageRegistry empty = %q, want empty", h.buildRepo)
	}

	// Leading/trailing slash trimmed
	h.SetBuildImageRegistry("/registry.example.com/team/")
	if h.buildRepo != "registry.example.com/team" {
		t.Errorf("SetBuildImageRegistry with slashes = %q", h.buildRepo)
	}
}

func TestDeployTriggerHandler_SetBuildImagePush(t *testing.T) {
	h := &DeployTriggerHandler{}

	h.SetBuildImagePush(true)
	if !h.buildPush {
		t.Error("SetBuildImagePush(true): expected true")
	}

	h.SetBuildImagePush(false)
	if h.buildPush {
		t.Error("SetBuildImagePush(false): expected false")
	}
}

func TestDeployTriggerHandler_SetBuildRegistryAuth(t *testing.T) {
	h := &DeployTriggerHandler{}

	h.SetBuildRegistryAuth("user1", "pass1")
	if h.buildUser != "user1" || h.buildPass != "pass1" {
		t.Errorf("SetBuildRegistryAuth got (%q, %q), want (user1, pass1)", h.buildUser, h.buildPass)
	}
}

func TestDeployTriggerHandler_failReservedDeployment(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Status:   "deploying",
	})
	h := &DeployTriggerHandler{store: store}

	// Test with nil dep
	h.failReservedDeployment(context.Background(), "app1", "tenant1", nil)
	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status 'failed', got %q", store.updatedStatus["app1"])
	}

	// Reset and test with valid dep — should mark deployment as failed
	store.updatedStatus["app1"] = ""
	dep := &core.Deployment{ID: "dep1", AppID: "app1", Status: "deploying"}
	store.deploymentsByApp["app1"] = []core.Deployment{*dep}
	h.failReservedDeployment(context.Background(), "app1", "tenant1", dep)
	if dep.Status != "failed" {
		t.Errorf("expected dep status 'failed', got %q", dep.Status)
	}
	if dep.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
}

func TestDeployTriggerHandler_failReserved(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Status:   "deploying",
	})
	h := &DeployTriggerHandler{store: store, events: events}

	dep := &core.Deployment{ID: "dep1", AppID: "app1", Status: "deploying"}
	h.failReserved(context.Background(), "app1", "tenant1", dep, "something went wrong")

	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status 'failed', got %q", store.updatedStatus["app1"])
	}
	if dep.Status != "failed" {
		t.Errorf("expected dep status 'failed', got %q", dep.Status)
	}
}

func TestDeployTriggerHandler_publishDeployFailed(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := &DeployTriggerHandler{events: events}

	// Should not panic
	h.publishDeployFailed(context.Background(), "test", "app1", "error msg")
}

func TestDeployTriggerHandler_publishDeployFailed_NilEvents(t *testing.T) {
	h := &DeployTriggerHandler{}

	// Should not panic with nil events
	h.publishDeployFailed(context.Background(), "test", "app1", "error msg")
}

func TestDeployTriggerHandler_failApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
	})
	store.errUpdateAppStatus = errors.New("store error")
	h := &DeployTriggerHandler{store: store}

	// Should not panic when store.UpdateAppStatus fails
	h.failApp(context.Background(), "app1", "tenant1")
}

func TestDeployTriggerHandler_failReservedDeployment_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
	})
	store.errUpdateAppStatus = errors.New("store error")
	h := &DeployTriggerHandler{store: store}

	// Should not panic when UpdateAppStatus fails
	dep := &core.Deployment{ID: "dep1", AppID: "app1"}
	h.failReservedDeployment(context.Background(), "app1", "tenant1", dep)
}

func TestDeployTriggerHandler_deployRuntimeForApp_NodeGetError(t *testing.T) {
	h := NewDeployTriggerHandler(newMockStore(), nil, nil)
	nm := &fakeNodeManager{nodes: map[string]core.NodeExecutor{}}
	h.SetNodeManager(nm)

	_, err := h.deployRuntimeForApp(&core.Application{ID: "app1", ServerID: "remote-1"})
	if err == nil {
		t.Fatal("expected error for disconnected remote server")
	}
	if !strings.Contains(err.Error(), "is not connected") {
		t.Errorf("error = %q, want 'is not connected'", err.Error())
	}
}

func TestDeployTriggerHandler_deployRuntimeForApp_NilRuntime(t *testing.T) {
	h := NewDeployTriggerHandler(newMockStore(), nil, nil)

	_, err := h.deployRuntimeForApp(&core.Application{ID: "app1"})
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if !strings.Contains(err.Error(), "container runtime not available") {
		t.Errorf("error = %q, want 'container runtime not available'", err.Error())
	}
}

func TestDeployTriggerHandler_cleanupPreviousAppContainers_NilRuntime(t *testing.T) {
	h := &DeployTriggerHandler{}
	// Should not panic with nil runtime
	h.cleanupPreviousAppContainers(context.Background(), nil, "app1", "keep")
}

func TestDeployTriggerHandler_cleanupPreviousAppContainers_StopError(t *testing.T) {
	rt := &errorInjectingRuntime{
		recordingDeployRuntime: recordingDeployRuntime{
			containers: []core.ContainerInfo{{ID: "old-1"}},
		},
		stopErr: errors.New("stop failed"),
	}
	NewDeployTriggerHandler(newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), rt, "app1", "keep")
	// Should continue even after stop error — remove should still be called
	if len(rt.removed) != 1 {
		t.Errorf("expected 1 remove call despite stop error, got %d", len(rt.removed))
	}
}

func TestDeployTriggerHandler_cleanupPreviousAppContainers_RemoveError(t *testing.T) {
	rt := &errorInjectingRuntime{
		recordingDeployRuntime: recordingDeployRuntime{
			containers: []core.ContainerInfo{{ID: "old-1"}},
		},
		removeErr: errors.New("remove failed"),
	}
	NewDeployTriggerHandler(newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), rt, "app1", "")
}

func TestBuildImageTagForRegistry_EmptyCommitSHA(t *testing.T) {
	// When commitSHA is empty, it falls back to GenerateID
	tag := buildImageTagForRegistry("registry.example.com", &core.Application{Name: "My App", ID: "app1"}, "")
	if !strings.HasPrefix(tag, "registry.example.com/my-app:") {
		t.Errorf("unexpected tag format: %q", tag)
	}
	if len(tag) <= len("registry.example.com/my-app:") {
		t.Errorf("tag too short: %q", tag)
	}
}

func TestImageNamePart_EdgeCases(t *testing.T) {
	// Empty name with fallback
	part := imageNamePart("", "Fallback-App")
	if part != "fallback-app" {
		t.Errorf("empty name with fallback = %q, want fallback-app", part)
	}

	// Only special characters
	part = imageNamePart("___", "default")
	if part == "" || part == "___" {
		t.Errorf("special chars only should produce fallback, got %q", part)
	}

	// Trailing separator
	part = imageNamePart("app-", "fallback")
	if strings.HasSuffix(part, "-") {
		t.Errorf("trailing dash should be trimmed: %q", part)
	}
	if part != "app" {
		t.Errorf("trailing dash trimmed = %q, want app", part)
	}

	// Uppercase and mixed separators
	part = imageNamePart("My_App.Name-Version", "fallback")
	if part != "my-app-name-version" {
		t.Errorf("mixed separators = %q, want my-app-name-version", part)
	}
}

func TestImageNamePart_EmptyNameAndEmptyFallback(t *testing.T) {
	// When both name and fallback are empty, should generate a fallback with prefix
	part := imageNamePart("", "")
	if !strings.HasPrefix(part, "app-") {
		t.Errorf("should generate 'app-' prefix, got %q", part)
	}
}

// ─── auth.go coverage ────────────────────────────────────────────────────────

func TestAuthHandler_SetLogger(t *testing.T) {
	h := &AuthHandler{}

	// Set logger
	customLogger := slog.New(slog.NewTextHandler(nil, nil))
	h.SetLogger(customLogger)
	if h.logger != customLogger {
		t.Error("SetLogger did not set logger")
	}

	// Set nil logger — should not replace existing
	h.SetLogger(nil)
	if h.logger != customLogger {
		t.Error("SetLogger(nil) should be a no-op")
	}
}

func TestAuthHandler_Log_NilLogger(t *testing.T) {
	h := &AuthHandler{logger: nil}
	// Should return slog.Default() rather than panic
	l := h.log()
	if l == nil {
		t.Fatal("log() returned nil")
	}
}

func TestAuthHandler_NewAuthHandler_NilAuthMod(t *testing.T) {
	h := NewAuthHandler(nil, newMockStore(), nil)
	if h.authMod != nil {
		t.Error("expected nil authMod")
	}
	if h.totpValidator != nil {
		t.Error("expected nil totpValidator when authMod is nil")
	}
}

func TestAuthHandler_NewAuthHandler_WithAuthMod(t *testing.T) {
	// Auth module with TOTP service
	mod := &testAuthServices{jwt: testJWT()}
	h := NewAuthHandler(mod, newMockStore(), nil)
	if h.authMod != mod {
		t.Error("expected authMod to be set")
	}
	// TOTP service is nil in testAuthServices, so totpValidator should be nil
	if h.totpValidator != nil {
		t.Error("expected nil totpValidator for nil TOTP service")
	}
}

func TestIsSecureRequest_EdgeCases(t *testing.T) {
	if isSecureRequest(nil) {
		t.Error("expected false for nil request")
	}

	// Request with X-Forwarded-Proto: https
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if !isSecureRequest(r) {
		t.Error("expected true for X-Forwarded-Proto: https")
	}

	// Request with X-Forwarded-Proto: http
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("X-Forwarded-Proto", "http")
	if isSecureRequest(r2) {
		t.Error("expected false for X-Forwarded-Proto: http")
	}
}

func TestGenerateSlug_EmptyOrSpecial(t *testing.T) {
	// Empty -> should generate fallback
	slug := generateSlug("")
	if slug == "" {
		t.Error("generateSlug('') should not be empty")
	}
	if len(slug) != 8 {
		t.Errorf("generateSlug('') length = %d, want 8", len(slug))
	}

	// Only special chars
	slug = generateSlug("___!!!")
	if slug == "" {
		t.Error("generateSlug('___!!!') should not be empty")
	}
}

func TestRegistrationTenantSlug_Truncation(t *testing.T) {
	// Name longer than 80 chars
	longName := strings.Repeat("a", 100)
	slug := registrationTenantSlug(longName)
	// Should start with truncated base (80 chars)
	if !strings.HasPrefix(slug, strings.Repeat("a", 80)) {
		t.Errorf("slug = %q, expected prefix of 80 a's", slug)
	}
	// Should contain a dash and short ID
	if !strings.Contains(slug, "-") {
		t.Errorf("slug = %q, expected '-' separator", slug)
	}
}

func TestRegistrationTenantSlug_NormalName(t *testing.T) {
	slug := registrationTenantSlug("My Team")
	if !strings.HasPrefix(slug, "my-team-") {
		t.Errorf("slug = %q, want prefix my-team-", slug)
	}
}

// ─── helpers.go coverage ─────────────────────────────────────────────────────

func TestWriteJSON_EncodeError(t *testing.T) {
	// writeJSON with a value that fails to encode (e.g., a channel)
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{"ch": make(chan int)})
	// Should not panic, should return 200
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPaginateSlice_EdgeCases(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	// Offset past total
	result, total := paginateSlice(items, pagination{Page: 10, PerPage: 20, Offset: 200})
	if total != 5 {
		t.Errorf("total = %d, want 5", result)
	}
	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}

	// Offset at exact boundary (should return empty slice, not panic)
	result, total = paginateSlice(items, pagination{Page: 2, PerPage: 5, Offset: 5})
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}

	// Empty slice
	result, total = paginateSlice([]int{}, pagination{Page: 1, PerPage: 20, Offset: 0})
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}
}

func TestParsePagination_Caps(t *testing.T) {
	// Page > maxPage
	r := httptest.NewRequest("GET", "/api/v1/apps?page=99999", nil)
	p := parsePagination(r)
	if p.Page != maxPage {
		t.Errorf("page = %d, want %d", p.Page, maxPage)
	}

	// per_page > 100
	r2 := httptest.NewRequest("GET", "/api/v1/apps?per_page=500", nil)
	p2 := parsePagination(r2)
	if p2.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", p2.PerPage)
	}

	// per_page < 1
	r3 := httptest.NewRequest("GET", "/api/v1/apps?per_page=-1", nil)
	p3 := parsePagination(r3)
	if p3.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", p3.PerPage)
	}

	// page < 1
	r4 := httptest.NewRequest("GET", "/api/v1/apps?page=0", nil)
	p4 := parsePagination(r4)
	if p4.Page != 1 {
		t.Errorf("page = %d, want 1", p4.Page)
	}
}

// ─── apps.go coverage ──────────────────────────────────────────────────────

func TestBuiltinPlanAppLimit_Found(t *testing.T) {
	limit, ok := builtinPlanAppLimit("free")
	if !ok {
		t.Error("expected 'free' plan to be found")
	}
	_ = limit
}

func TestBuiltinPlanAppLimit_NotFound(t *testing.T) {
	limit, ok := builtinPlanAppLimit("nonexistent-plan")
	if ok {
		t.Error("expected 'nonexistent-plan' to not be found")
	}
	if limit != 0 {
		t.Errorf("limit = %d, want 0", limit)
	}
}

func TestStricterPositiveLimit_EdgeCases(t *testing.T) {
	// a <= 0
	if got := stricterPositiveLimit(0, 5); got != 5 {
		t.Errorf("stricterPositiveLimit(0,5) = %d, want 5", got)
	}
	if got := stricterPositiveLimit(-1, 5); got != 5 {
		t.Errorf("stricterPositiveLimit(-1,5) = %d, want 5", got)
	}

	// b <= 0
	if got := stricterPositiveLimit(5, 0); got != 5 {
		t.Errorf("stricterPositiveLimit(5,0) = %d, want 5", got)
	}
	if got := stricterPositiveLimit(5, -1); got != 5 {
		t.Errorf("stricterPositiveLimit(5,-1) = %d, want 5", got)
	}

	// both positive, a < b
	if got := stricterPositiveLimit(3, 10); got != 3 {
		t.Errorf("stricterPositiveLimit(3,10) = %d, want 3", got)
	}

	// both positive, a > b
	if got := stricterPositiveLimit(10, 3); got != 3 {
		t.Errorf("stricterPositiveLimit(10,3) = %d, want 3", got)
	}

	// equal
	if got := stricterPositiveLimit(5, 5); got != 5 {
		t.Errorf("stricterPositiveLimit(5,5) = %d, want 5", got)
	}
}

func TestFindAppContainerID_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices()
	// Services.Container is nil by default
	h := NewAppHandler(newMockStore(), c)

	r := httptest.NewRequest("GET", "/", nil)
	id, err := h.findAppContainerID(r, "app1")
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if id != "" {
		t.Errorf("id = %q, want empty", id)
	}
}

func TestFindAppContainerID_EmptyContainerList(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	h := NewAppHandler(newMockStore(), c)

	r := httptest.NewRequest("GET", "/", nil)
	id, err := h.findAppContainerID(r, "app1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("id = %q, want empty (no containers)", id)
	}
}

func TestAppHandler_Restart_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices() // Container is nil
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Restart(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Stop_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Stop(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Start_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Start(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Stop_ContainerLookupError(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{listErr: errors.New("list failed")}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Stop(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Stop_NoContainer(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Stop(rr, req)

	// Idempotent stop on undeployed app
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.updatedStatus["app1"] != "stopped" {
		t.Errorf("expected app status 'stopped', got %q", store.updatedStatus["app1"])
	}
}

func TestAppHandler_Restart_ContainerLookupError(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{listErr: errors.New("list failed")}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Restart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Restart_NoContainer(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Restart(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Start_ContainerLookupError(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{listErr: errors.New("list failed")}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Start(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Start_NoContainer(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Start(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── errorInjectingRuntime wraps recordingDeployRuntime to inject stop/remove errors ─

type errorInjectingRuntime struct {
	recordingDeployRuntime
	stopErr   error
	removeErr error
}

func (r *errorInjectingRuntime) Stop(ctx context.Context, id string, timeout int) error {
	r.stopped = append(r.stopped, id)
	if r.stopErr != nil {
		return r.stopErr
	}
	return nil
}

func (r *errorInjectingRuntime) Remove(ctx context.Context, id string, force bool) error {
	r.removed = append(r.removed, id)
	if r.removeErr != nil {
		return r.removeErr
	}
	return nil
}

// ─── backups.go coverage ─────────────────────────────────────────────────────

func TestBackupHandler_SetLogger(t *testing.T) {
	h := &BackupHandler{}
	customLogger := slog.New(slog.NewTextHandler(nil, nil))
	h.SetLogger(customLogger)
	if h.logger != customLogger {
		t.Error("SetLogger did not set logger")
	}
}

func TestIsStrictBackupKey_EdgeCases(t *testing.T) {
	// Double-encoded path
	if isStrictBackupKey("tenant1%252Fbackup") {
		t.Error("expected false for double-encoded key")
	}

	// Key with double slash
	if isStrictBackupKey("tenant1//backup") {
		t.Error("expected false for double slash")
	}

	// Key with trailing slash
	if isStrictBackupKey("tenant1/backup/") {
		t.Error("expected false for trailing slash")
	}

	// Key with invalid characters
	if isStrictBackupKey("tenant1/backup<file>") {
		t.Error("expected false for invalid characters")
	}

	// Valid key
	if !isStrictBackupKey("tenant1/backup-file_v1.2") {
		t.Error("expected true for valid key")
	}

	// Key with URL encoding is rejected because decoded != key
	if isStrictBackupKey("tenant1/backup%20file") {
		t.Error("expected false for URL-encoded key (decoded != key check)")
	}

	// Underscore, hyphens, dots all valid
	if !isStrictBackupKey("tenant1/backup-file_v1.2") {
		t.Error("expected true for valid key")
	}

	// Empty key component after split
	if isStrictBackupKey("tenant1//backup") {
		t.Error("expected false for empty path component")
	}
}

// ─── commands.go coverage ────────────────────────────────────────────────────

func TestCommandHandler_SetBolt(t *testing.T) {
	h := &CommandHandler{}
	mockBolt := newMockBoltStore()
	h.SetBolt(mockBolt)
	if h.bolt != mockBolt {
		t.Error("SetBolt did not set bolt")
	}
}

func TestCommandHandler_History_NilBolt(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := &CommandHandler{store: store}

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCommandHandler_History_BoltListError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	bolt := newMockBoltStore()
	// Don't add any data — List returns error for missing bucket
	h := &CommandHandler{store: store, bolt: bolt}

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.History(rr, req)

	// Should return empty list, not error
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCommandHandler_History_NonMatchingPrefix(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	bolt := newMockBoltStore()
	// Seed a command for a different app
	_ = bolt.Set("app_commands", "app2:cmd1", commandHistoryEntry{
		ID: "cmd1", AppID: "app2",
	}, 3600)
	h := &CommandHandler{store: store, bolt: bolt}

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── event_webhooks.go coverage ──────────────────────────────────────────────

func TestValidateWebhookURL_EdgeCases(t *testing.T) {
	// Empty URL
	if err := validateWebhookURL(""); err == nil {
		t.Error("expected error for empty URL")
	}

	// Invalid URL
	if err := validateWebhookURL("://invalid"); err == nil {
		t.Error("expected error for invalid URL")
	}

	// Non-HTTPS scheme
	if err := validateWebhookURL("http://example.com/webhook"); err == nil {
		t.Error("expected error for non-HTTPS URL")
	}

	// Empty hostname
	if err := validateWebhookURL("https:///path"); err == nil {
		t.Error("expected error for empty hostname")
	}

	// Localhost
	if err := validateWebhookURL("https://localhost/webhook"); err == nil {
		t.Error("expected error for localhost")
	}

	// IP loopback
	if err := validateWebhookURL("https://127.0.0.1/webhook"); err == nil {
		t.Error("expected error for loopback IP")
	}

	// Cloud metadata IP
	if err := validateWebhookURL("https://169.254.169.254/"); err == nil {
		t.Error("expected error for cloud metadata IP")
	}

	// Internal hostname
	if err := validateWebhookURL("https://metadata.google.internal/"); err == nil {
		t.Error("expected error for internal hostname")
	}

	// Subdomain of internal hostname
	if err := validateWebhookURL("https://sub.metadata.google.internal/"); err == nil {
		t.Error("expected error for subdomain of internal hostname")
	}

	// Valid URL
	if err := validateWebhookURL("https://example.com/webhook"); err != nil {
		t.Errorf("unexpected error for valid URL: %v", err)
	}
}

// mutateBoltValue covers the non-Mutate path (fallback read+write).
func TestMutateBoltValue_NonMutatorBolt(t *testing.T) {
	bolt := newMockBoltStore()
	// prime data
	var dest struct {
		Count int `json:"count"`
	}
	err := mutateBoltValue(bolt, "test_bucket", "test_key", &dest, 3600, func(exists bool) error {
		if !exists {
			return errors.New("expected exists")
		}
		dest.Count = 42
		return nil
	})
	if err == nil {
		t.Error("expected error because key does not exist yet")
	}
}

func TestMutateBoltValue_NonMutatorBolt_NewKey(t *testing.T) {
	bolt := newMockBoltStore()
	var dest struct {
		Count int `json:"count"`
	}
	err := mutateBoltValue(bolt, "test_bucket", "new_key", &dest, 3600, func(exists bool) error {
		if exists {
			return errors.New("expected not exists")
		}
		dest.Count = 99
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify value was stored
	var got struct {
		Count int `json:"count"`
	}
	if err := bolt.Get("test_bucket", "new_key", &got); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Count != 99 {
		t.Errorf("count = %d, want 99", got.Count)
	}
}

func TestMutateBoltValue_NonMutatorBolt_MutateReturnsError(t *testing.T) {
	bolt := newMockBoltStore()
	var dest struct{}
	err := mutateBoltValue(bolt, "test_bucket", "new_key", &dest, 3600, func(exists bool) error {
		return errors.New("mutate failed")
	})
	if err == nil {
		t.Error("expected error from mutate callback")
	}
}

// ─── billing.go coverage ─────────────────────────────────────────────────────

func TestUsageRecordTime_MissingFields(t *testing.T) {
	rec := core.UsageRecord{
		TenantID: "t1",
		// No HourBucket or CreatedAt set
	}
	result := usageRecordTime(rec)
	if !result.IsZero() {
		t.Error("expected zero time when both HourBucket and CreatedAt are zero")
	}
}

func TestUsageRecordTime_HourBucket(t *testing.T) {
	now := time.Now()
	rec := core.UsageRecord{
		TenantID:   "t1",
		HourBucket: now,
	}
	result := usageRecordTime(rec)
	if !result.Equal(now) {
		t.Errorf("usageRecordTime = %v, want %v", result, now)
	}
}

func TestUsageRecordTime_FallsBackToCreatedAt(t *testing.T) {
	now := time.Now()
	rec := core.UsageRecord{
		TenantID:  "t1",
		CreatedAt: now,
	}
	result := usageRecordTime(rec)
	if !result.Equal(now) {
		t.Errorf("usageRecordTime = %v, want %v", result, now)
	}
}

// ─── sessions.go coverage ────────────────────────────────────────────────────

func TestSessionHandler_GetTOTPStatus_NilAuthMod(t *testing.T) {
	h := &SessionHandler{} // authMod is nil
	req := httptest.NewRequest("GET", "/api/v1/auth/totp/status", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.GetTOTPStatus(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSessionHandler_DisableTOTP_NilAuthMod(t *testing.T) {
	h := &SessionHandler{} // authMod is nil
	req := httptest.NewRequest("POST", "/api/v1/auth/totp/disable", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.DisableTOTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSessionHandler_GenerateBackupCodes_NilAuthMod(t *testing.T) {
	h := &SessionHandler{} // authMod is nil
	req := httptest.NewRequest("POST", "/api/v1/auth/totp/backup-codes", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.GenerateBackupCodes(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── certificates.go coverage ────────────────────────────────────────────────

// TestCertMatchesDomain covers wildcard matching and SAN/CN fallback.
func TestCertMatchesDomain(t *testing.T) {
	// Wildcard cert matching subdomain
	cert := &x509.Certificate{
		DNSNames: []string{"*.example.com"},
	}
	if !certMatchesDomain(cert, "sub.example.com") {
		t.Error("expected wildcard *.example.com to match sub.example.com")
	}
	if certMatchesDomain(cert, "example.com") {
		t.Error("expected wildcard *.example.com to NOT match example.com")
	}
	if certMatchesDomain(cert, "other.com") {
		t.Error("expected wildcard *.example.com to NOT match other.com")
	}

	// SAN exact match
	cert2 := &x509.Certificate{
		DNSNames: []string{"app.example.com"},
	}
	if !certMatchesDomain(cert2, "app.example.com") {
		t.Error("expected exact SAN match")
	}

	// CN fallback (no SANs)
	cert3 := &x509.Certificate{
		Subject: pkix.Name{CommonName: "cn.example.com"},
	}
	if !certMatchesDomain(cert3, "cn.example.com") {
		t.Error("expected CN fallback match")
	}
	if certMatchesDomain(cert3, "wrong.example.com") {
		t.Error("expected CN fallback to NOT match wrong domain")
	}

	// No match at all
	if certMatchesDomain(cert2, "nonexistent.com") {
		t.Error("expected no match for unrelated domain")
	}
}

// ─── auth.go Login coverage — edge case: nil claims context ──────────────────

func TestLoginRateLimitCheck_NilBolt(t *testing.T) {
	h := &AuthHandler{} // bolt is nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	result := h.loginRateLimitCheck(w, r, "test@example.com")
	if result != 0 {
		t.Errorf("expected 0 for nil bolt, got %d", result)
	}
}

func TestCheckPerAccountRateLimit_NotFoundError(t *testing.T) {
	h := &AuthHandler{bolt: newMockBoltStore()}
	locked, until := h.checkPerAccountRateLimit("nonexistent@example.com")
	if locked {
		t.Error("expected not locked for nonexistent email")
	}
	if until != 0 {
		t.Errorf("expected until=0, got %d", until)
	}
}

// ─── deploy_trigger.go deployGitApp nil runtime path ─────────────────────────

func TestDeployTriggerHandler_deployGitApp_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Status:   "idle",
	})
	h := &DeployTriggerHandler{store: store} // runtime is nil

	err := h.deployGitApp(context.Background(), store.apps["app1"], "manual", "", nil)
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if !strings.Contains(err.Error(), "container runtime not available") {
		t.Errorf("error = %q, want 'container runtime not available'", err.Error())
	}
	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status 'failed', got %q", store.updatedStatus["app1"])
	}
}
