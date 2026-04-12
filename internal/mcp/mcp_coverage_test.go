package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// deployApp error branches — covers handler.go:100 (77.8% → ~95%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestDeployApp_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	resp, err := h.HandleToolCall(context.Background(), "deploy_app", json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestDeployApp_MissingRequiredFields(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{"name": "app"}) // missing source_type, source_url
	resp, err := h.HandleToolCall(context.Background(), "deploy_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "required") {
		t.Error("expected error about required fields")
	}
}

func TestDeployApp_NoTenantAvailable(t *testing.T) {
	store := &mockStore{}
	// Override ListAllTenants to return empty
	h := NewHandler(&mockStoreNoTenants{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	_ = store
	input, _ := json.Marshal(map[string]string{
		"name":        "app",
		"source_type": "git",
		"source_url":  "https://github.com/test/test",
	})
	resp, err := h.HandleToolCall(context.Background(), "deploy_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "tenant") {
		t.Error("expected error about no tenant available")
	}
}

func TestDeployApp_CreateAppError(t *testing.T) {
	store := &mockStoreCreateAppErr{createErr: fmt.Errorf("duplicate name")}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"name":        "app",
		"source_type": "git",
		"source_url":  "https://github.com/test/test",
		"tenant_id":   "t-1",
	})
	resp, err := h.HandleToolCall(context.Background(), "deploy_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "failed to create") {
		t.Error("expected create app error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// createDatabase error branches — covers handler.go:208 (66.7% → ~95%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestCreateDatabase_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	resp, err := h.HandleToolCall(context.Background(), "create_database", json.RawMessage(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestCreateDatabase_MissingFields(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{"engine": "mysql"}) // missing name
	resp, err := h.HandleToolCall(context.Background(), "create_database", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "required") {
		t.Error("expected error about required fields")
	}
}

func TestCreateDatabase_AppNotFound(t *testing.T) {
	store := &mockStore{appErr: fmt.Errorf("not found")}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"engine": "postgres",
		"name":   "mydb",
		"app_id": "nonexistent",
	})
	resp, err := h.HandleToolCall(context.Background(), "create_database", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "not found") {
		t.Error("expected app not found error")
	}
}

func TestCreateDatabase_UnsupportedEngine(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"engine": "oracle",
		"name":   "mydb",
	})
	resp, err := h.HandleToolCall(context.Background(), "create_database", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "Unsupported") {
		t.Error("expected unsupported engine error")
	}
}

func TestCreateDatabase_AllEngines(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())

	engines := []struct {
		engine string
		prefix string
	}{
		{"mysql", "mysql://"},
		{"postgres", "postgres://"},
		{"redis", "redis://"},
		{"mongodb", "mongodb://"},
	}

	for _, tt := range engines {
		t.Run(tt.engine, func(t *testing.T) {
			input, _ := json.Marshal(map[string]string{
				"engine": tt.engine,
				"name":   "testdb",
			})
			resp, err := h.HandleToolCall(context.Background(), "create_database", input)
			if err != nil {
				t.Fatal(err)
			}
			if resp.IsError {
				t.Errorf("unexpected error: %v", resp.Content)
			}
			if !strings.Contains(resp.Content[0].Text, tt.prefix) {
				t.Errorf("response should contain %s connection string", tt.prefix)
			}
		})
	}
}

func TestCreateDatabase_DefaultUserPassword(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"engine": "mysql",
		"name":   "testdb",
		// no user, no password → defaults
	})
	resp, err := h.HandleToolCall(context.Background(), "create_database", input)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsError {
		t.Errorf("unexpected error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "dbuser") {
		t.Error("expected default user 'dbuser'")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// addDomain error branches — covers handler.go:275 (78.9% → ~95%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestAddDomain_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	resp, err := h.HandleToolCall(context.Background(), "add_domain", json.RawMessage(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestAddDomain_MissingFields(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{"app_id": "app-1"}) // missing fqdn
	resp, err := h.HandleToolCall(context.Background(), "add_domain", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "required") {
		t.Error("expected error about required fields")
	}
}

func TestAddDomain_AppNotFound(t *testing.T) {
	store := &mockStore{appErr: fmt.Errorf("not found")}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"app_id": "nonexistent",
		"fqdn":   "example.com",
	})
	resp, err := h.HandleToolCall(context.Background(), "add_domain", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "not found") {
		t.Error("expected app not found error")
	}
}

func TestAddDomain_CreateDomainError(t *testing.T) {
	store := &mockStoreCreateDomainErr{
		app:       &core.Application{ID: "app-1", Name: "web"},
		domainErr: fmt.Errorf("duplicate fqdn"),
	}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"app_id": "app-1",
		"fqdn":   "example.com",
	})
	resp, err := h.HandleToolCall(context.Background(), "add_domain", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "Failed to create domain") {
		t.Error("expected create domain error")
	}
}

func TestAddDomain_DefaultTypeAndProvider(t *testing.T) {
	store := &mockStore{
		app: &core.Application{ID: "app-1", Name: "web"},
	}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"app_id": "app-1",
		"fqdn":   "test.com",
		// no type or dns_provider → defaults
	})
	resp, err := h.HandleToolCall(context.Background(), "add_domain", input)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsError {
		t.Errorf("unexpected error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "custom") {
		t.Error("expected default type 'custom'")
	}
	if !strings.Contains(resp.Content[0].Text, "manual") {
		t.Error("expected default dns_provider 'manual'")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// marketplaceDeploy error branches — covers handler.go:337 (68.2% → ~95%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestMarketplaceDeploy_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	resp, err := h.HandleToolCall(context.Background(), "marketplace_deploy", json.RawMessage(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestMarketplaceDeploy_MissingFields(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{"template_slug": "nginx"}) // missing name
	resp, err := h.HandleToolCall(context.Background(), "marketplace_deploy", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "required") {
		t.Error("expected error about required fields")
	}
}

func TestMarketplaceDeploy_NoTenant(t *testing.T) {
	h := NewHandler(&mockStoreNoTenants{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"template_slug": "nginx",
		"name":          "my-nginx",
	})
	resp, err := h.HandleToolCall(context.Background(), "marketplace_deploy", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "tenant") {
		t.Error("expected error about no tenant")
	}
}

func TestMarketplaceDeploy_CreateAppError(t *testing.T) {
	store := &mockStoreCreateAppErr{createErr: fmt.Errorf("db error")}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"template_slug": "nginx",
		"name":          "my-nginx",
		"tenant_id":     "t-1",
	})
	resp, err := h.HandleToolCall(context.Background(), "marketplace_deploy", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "failed to create") {
		t.Error("expected create app error")
	}
}

func TestMarketplaceDeploy_WithDomain(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"template_slug": "wordpress",
		"name":          "my-wp",
		"tenant_id":     "t-1",
		"domain":        "blog.example.com",
	})
	resp, err := h.HandleToolCall(context.Background(), "marketplace_deploy", input)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsError {
		t.Errorf("unexpected error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "blog.example.com") {
		t.Error("response should include domain")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// provisionServer error branches — covers handler.go:409 (82.4% → ~95%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestProvisionServer_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	resp, err := h.HandleToolCall(context.Background(), "provision_server", json.RawMessage(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestProvisionServer_MissingFields(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{"provider": "hetzner"}) // missing name
	resp, err := h.HandleToolCall(context.Background(), "provision_server", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "required") {
		t.Error("expected error about required fields")
	}
}

func TestProvisionServer_UnsupportedProvider(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"provider": "azure",
		"name":     "my-server",
	})
	resp, err := h.HandleToolCall(context.Background(), "provision_server", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "Unsupported") {
		t.Error("expected unsupported provider error")
	}
}

func TestProvisionServer_DefaultRegionAndSize(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]string{
		"provider": "digitalocean",
		"name":     "web-1",
		// no region, no size → defaults
	})
	resp, err := h.HandleToolCall(context.Background(), "provision_server", input)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsError {
		t.Errorf("unexpected error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "auto") {
		t.Error("expected default region 'auto'")
	}
	if !strings.Contains(resp.Content[0].Text, "small") {
		t.Error("expected default size 'small'")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Additional mock stores for specific error paths
// ═══════════════════════════════════════════════════════════════════════════════

// mockStoreNoTenants returns empty tenant list.
type mockStoreNoTenants struct {
	core.Store
}

func (m *mockStoreNoTenants) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}
func (m *mockStoreNoTenants) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (m *mockStoreNoTenants) CreateApp(_ context.Context, _ *core.Application) error { return nil }

// mockStoreCreateAppErr fails on CreateApp.
type mockStoreCreateAppErr struct {
	core.Store
	createErr error
}

func (m *mockStoreCreateAppErr) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return []core.Tenant{{ID: "t-1", Name: "Test"}}, 1, nil
}
func (m *mockStoreCreateAppErr) CreateApp(_ context.Context, _ *core.Application) error {
	return m.createErr
}

// mockStoreCreateDomainErr fails on CreateDomain.
type mockStoreCreateDomainErr struct {
	core.Store
	app       *core.Application
	domainErr error
}

func (m *mockStoreCreateDomainErr) GetApp(_ context.Context, _ string) (*core.Application, error) {
	if m.app != nil {
		return m.app, nil
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockStoreCreateDomainErr) CreateDomain(_ context.Context, _ *core.Domain) error {
	return m.domainErr
}
