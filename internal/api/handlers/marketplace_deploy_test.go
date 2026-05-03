package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// newDeployTestRegistry returns a registry with a realistic compose template
// and a build-only template (used to exercise the downstream deployer error
// path indirectly).
func newDeployTestRegistry() *marketplace.TemplateRegistry {
	reg := marketplace.NewTemplateRegistry()
	reg.Add(&marketplace.Template{
		Slug:        "wordpress",
		Name:        "WordPress",
		Category:    "cms",
		Description: "The world's most popular CMS",
		Tags:        []string{"blog", "cms"},
		Version:     "6.7",
		Author:      "WordPress.org",
		Verified:    true,
		Featured:    true,
		ComposeYAML: `services:
  wordpress:
    image: wordpress:6.7-apache
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: wordpress
      WORDPRESS_DB_PASSWORD: ${DB_PASSWORD:-changeme}
  db:
    image: mariadb:11
    environment:
      MARIADB_DATABASE: wordpress
      MARIADB_PASSWORD: ${DB_PASSWORD:-changeme}
`,
	})
	reg.Add(&marketplace.Template{
		Slug:        "broken-yaml",
		Name:        "BrokenYAML",
		Category:    "test",
		Description: "intentionally broken",
		Version:     "1",
		Author:      "test",
		// Tab characters at the start of a line make YAML parsers unhappy
		// and `services:` missing altogether makes compose.Parse reject it.
		ComposeYAML: "\t: this : is : not : valid : yaml\nservices: [not a map]",
	})
	return reg
}

// newDeployHandler wires a deploy handler with a mock store, a mock runtime,
// and a real EventBus — the one the handler's async goroutine publishes into.
func newDeployHandler(store core.Store) *MarketplaceDeployHandler {
	runtime := &mockContainerRuntime{}
	events := core.NewEventBus(nil)
	reg := newDeployTestRegistry()
	return NewMarketplaceDeployHandler(reg, runtime, store, events)
}

// newDeployRequest builds an authed POST /api/v1/marketplace/deploy request
// with JSON body and seeded JWT claims.
func newDeployRequest(t *testing.T, body any, tenantID, userID string) *http.Request {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/marketplace/deploy", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	return withClaims(req, userID, tenantID, "admin", "admin@test.local")
}

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestMarketplaceDeploy_Success is the happy-path: valid template, authed
// user, async deploy finishes without error → app record persisted, status
// 202 Accepted, body reports deploying + service count.
func TestMarketplaceDeploy_Success(t *testing.T) {
	store := newMockStore()
	// Seed a default project so the handler attaches the new app to it.
	store.addProject("tenant-1", core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "default"})

	h := newDeployHandler(store)
	h.SetServerContext(context.Background())

	req := newDeployRequest(t, map[string]any{
		"slug": "wordpress",
		"name": "my-blog",
		"config": map[string]string{
			"DB_PASSWORD": "strongpass123",
		},
	}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["template"] != "wordpress" {
		t.Errorf("template: got %v, want wordpress", resp["template"])
	}
	if resp["name"] != "my-blog" {
		t.Errorf("name: got %v, want my-blog", resp["name"])
	}
	if resp["status"] != "deploying" {
		t.Errorf("status: got %v, want deploying", resp["status"])
	}
	if svc, _ := resp["services"].(float64); int(svc) != 2 {
		t.Errorf("services: got %v, want 2", resp["services"])
	}
	if resp["app_id"] == "" || resp["app_id"] == nil {
		t.Error("expected app_id in response")
	}

	// Store recorded the new app with tenant + source type.
	if store.createdApp == nil {
		t.Fatal("expected CreateApp to be called")
	}
	if store.createdApp.TenantID != "tenant-1" {
		t.Errorf("created app tenant: got %q, want tenant-1", store.createdApp.TenantID)
	}
	if store.createdApp.SourceType != "marketplace" {
		t.Errorf("created app source_type: got %q, want marketplace", store.createdApp.SourceType)
	}
	if store.createdApp.Type != "compose-stack" {
		t.Errorf("created app type: got %q, want compose-stack", store.createdApp.Type)
	}
	if store.createdApp.ProjectID != "proj-1" {
		t.Errorf("created app project_id: got %q, want proj-1 (default project)", store.createdApp.ProjectID)
	}

	// Drain the async goroutine so we don't leak it into sibling tests.
	WaitForBackground()
}

// TestMarketplaceDeploy_NameDefaultsToSlug verifies the contract that an
// empty `name` field falls back to the template slug.
func TestMarketplaceDeploy_NameDefaultsToSlug(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)
	h.SetServerContext(context.Background())

	req := newDeployRequest(t, map[string]any{
		"slug":   "wordpress",
		"config": map[string]string{"DB_PASSWORD": "abc"},
	}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202: %s", rr.Code, rr.Body.String())
	}
	if store.createdApp == nil || store.createdApp.Name != "wordpress" {
		t.Errorf("expected name to default to slug 'wordpress', got %+v", store.createdApp)
	}
	WaitForBackground()
}

// TestMarketplaceDeploy_Unauthorized rejects requests without auth claims.
func TestMarketplaceDeploy_Unauthorized(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)

	body, _ := json.Marshal(map[string]any{"slug": "wordpress"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/marketplace/deploy", bytes.NewReader(body))
	// No withClaims call — context has no auth.Claims.
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
	if store.createdApp != nil {
		t.Error("CreateApp must not be called on unauthorized request")
	}
}

// TestMarketplaceDeploy_InvalidJSON rejects malformed request bodies.
func TestMarketplaceDeploy_InvalidJSON(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/marketplace/deploy", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	req = withClaims(req, "u", "t", "r", "e@x")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// TestMarketplaceDeploy_MissingSlug rejects requests without a template slug.
func TestMarketplaceDeploy_MissingSlug(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)

	req := newDeployRequest(t, map[string]any{"name": "my-app"}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rr.Code)
	}
	assertErrorMessage(t, rr, "slug is required")
}

// TestMarketplaceDeploy_TemplateNotFound returns 404 for unknown slugs.
func TestMarketplaceDeploy_TemplateNotFound(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)

	req := newDeployRequest(t, map[string]any{"slug": "does-not-exist"}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "template not found: does-not-exist")
	if store.createdApp != nil {
		t.Error("CreateApp must not be called for unknown template")
	}
}

// TestMarketplaceDeploy_InvalidComposeYAML surfaces a 400 when the template
// ships a broken compose file (the registry template itself is malformed).
func TestMarketplaceDeploy_InvalidComposeYAML(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)

	req := newDeployRequest(t, map[string]any{"slug": "broken-yaml"}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400: %s", rr.Code, rr.Body.String())
	}
	if store.createdApp != nil {
		t.Error("CreateApp must not be called when compose parse fails")
	}
}

// TestMarketplaceDeploy_CreateAppFails surfaces a 500 when the store rejects
// the new Application.
func TestMarketplaceDeploy_CreateAppFails(t *testing.T) {
	store := newMockStore()
	store.errCreateApp = errors.New("disk full")
	h := newDeployHandler(store)

	req := newDeployRequest(t, map[string]any{
		"slug":   "wordpress",
		"config": map[string]string{"DB_PASSWORD": "abc"},
	}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to create app")
}

// TestMarketplaceDeploy_NoDefaultProjectStillSucceeds covers the case where
// the tenant has no projects yet — the handler should still accept the
// request and create the app without a ProjectID.
func TestMarketplaceDeploy_NoDefaultProjectStillSucceeds(t *testing.T) {
	store := newMockStore()
	// Deliberately do NOT seed any project for tenant-1.
	h := newDeployHandler(store)
	h.SetServerContext(context.Background())

	req := newDeployRequest(t, map[string]any{
		"slug":   "wordpress",
		"name":   "no-proj-app",
		"config": map[string]string{"DB_PASSWORD": "abc"},
	}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202: %s", rr.Code, rr.Body.String())
	}
	if store.createdApp == nil {
		t.Fatal("expected CreateApp to be called")
	}
	if store.createdApp.ProjectID != "" {
		t.Errorf("expected empty ProjectID when no default project exists, got %q", store.createdApp.ProjectID)
	}
	WaitForBackground()
}

// TestMarketplaceDeploy_ConfigInterpolation verifies that the user-supplied
// `config` map is applied to the compose YAML — specifically, that placeholder
// substitution in the template reaches the downstream deployer. We do this
// indirectly: the compose parser does not fail, and the services count in
// the response matches what the interpolated YAML declares.
func TestMarketplaceDeploy_ConfigInterpolation(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)
	h.SetServerContext(context.Background())

	req := newDeployRequest(t, map[string]any{
		"slug": "wordpress",
		"name": "interp-test",
		"config": map[string]string{
			"DB_PASSWORD": "my-secret-pw",
		},
	}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if svc, _ := resp["services"].(float64); int(svc) != 2 {
		t.Errorf("services count after interpolation: got %v, want 2", resp["services"])
	}
	WaitForBackground()
}

func TestMarketplaceDeploy_ReturnsGeneratedSecrets(t *testing.T) {
	store := newMockStore()
	h := newDeployHandler(store)
	h.SetServerContext(context.Background())

	req := newDeployRequest(t, map[string]any{
		"slug": "wordpress",
		"name": "generated-secret-test",
	}, "tenant-1", "user-1")
	rr := httptest.NewRecorder()

	h.Deploy(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	generated, ok := resp["generated_secrets"].(map[string]any)
	if !ok {
		t.Fatalf("generated_secrets missing or wrong type: %#v", resp["generated_secrets"])
	}
	if generated["DB_PASSWORD"] == "" {
		t.Fatalf("expected generated DB_PASSWORD, got %#v", generated)
	}
	if generated["DB_PASSWORD"] == "changeme" {
		t.Fatal("generated DB_PASSWORD must not use weak fallback")
	}
	WaitForBackground()
}
