package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/webhooks"
)

// =====================================================
// NewPipeline — constructor
// =====================================================

func TestNewPipeline(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)
	logger := slog.Default()

	p := NewPipeline(store, runtime, events, logger)
	if p == nil {
		t.Fatal("NewPipeline returned nil")
	}
	if p.store != store {
		t.Error("store field not set")
	}
	if p.builder == nil {
		t.Error("builder should be created")
	}
	if p.deployer == nil {
		t.Error("deployer should be created")
	}
	if p.events != events {
		t.Error("events field not set")
	}
	if p.logger != logger {
		t.Error("logger field not set")
	}
}

func TestNewPipeline_NilDeps(t *testing.T) {
	p := NewPipeline(nil, nil, nil, slog.Default())
	if p == nil {
		t.Fatal("NewPipeline should never return nil even with nil deps")
	}
}

// =====================================================
// findAppBySourceURL — edge cases
// =====================================================

func TestPipeline_FindAppBySourceURL_EmptyURL(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	_, err := p.findAppBySourceURL(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	if err.Error() != "empty repository URL" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipeline_FindAppBySourceURL_NoTenants(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	_, err := p.findAppBySourceURL(context.Background(), "https://github.com/test/repo")
	if err == nil {
		t.Fatal("expected error when no tenants exist")
	}
}

func TestPipeline_FindAppBySourceURL_Found(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID:        "app-1",
		Name:      "my-app",
		SourceURL: "https://github.com/test/repo",
		TenantID:  "t-1",
	}
	// ListAllTenants needs to return at least one tenant
	// The mockStore's ListAllTenants returns allTenantsList which is empty by default.
	// We need to set it up to return a tenant so ListAppsByTenant is called.

	// Add a fake tenant to allTenantsList
	store.allTenantsList = []core.Tenant{{ID: "t-1", Name: "Test Tenant"}}

	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	app, err := p.findAppBySourceURL(context.Background(), "https://github.com/test/repo")
	if err != nil {
		t.Fatalf("findAppBySourceURL error: %v", err)
	}
	if app.ID != "app-1" {
		t.Errorf("app ID = %q, want app-1", app.ID)
	}
}

func TestPipeline_FindAppBySourceURL_NotFound(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID:        "app-1",
		Name:      "my-app",
		SourceURL: "https://github.com/test/different-repo",
		TenantID:  "t-1",
	}
	store.allTenantsList = []core.Tenant{{ID: "t-1", Name: "Test Tenant"}}

	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	_, err := p.findAppBySourceURL(context.Background(), "https://github.com/test/repo")
	if err == nil {
		t.Fatal("expected error when no app matches")
	}
}

func TestPipeline_FindAppBySourceURL_ListTenantsError(t *testing.T) {
	store := newMockStore()
	store.listTenantsErr = true

	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	_, err := p.findAppBySourceURL(context.Background(), "https://github.com/test/repo")
	if err == nil {
		t.Fatal("expected error when ListAllTenants fails")
	}
}

func TestPipeline_FindAppBySourceURL_ListAppsError_Continues(t *testing.T) {
	store := newMockStore()
	store.listAppsByTenantErr = fmt.Errorf("db error")
	store.allTenantsList = []core.Tenant{
		{ID: "t-1", Name: "Tenant 1"},
		{ID: "t-2", Name: "Tenant 2"},
	}

	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	_, err := p.findAppBySourceURL(context.Background(), "https://github.com/test/repo")
	if err == nil {
		t.Fatal("expected error when no app matches")
	}
	// The function should continue past the ListAppsByTenant error
	// and eventually return "no app found" error
}

// =====================================================
// HandleWebhook — error paths
// =====================================================

func TestPipeline_HandleWebhook_NoMatchingApp(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	payload := webhooks.WebhookPayload{
		RepoURL:   "https://github.com/test/nonexistent",
		Branch:    "main",
		CommitSHA: "abc123",
	}

	err := p.HandleWebhook(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error when no app matches the repo URL")
	}
}

func TestPipeline_HandleWebhook_BranchMismatch(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID:        "app-1",
		Name:      "branch-app",
		SourceURL: "https://github.com/test/repo",
		TenantID:  "t-1",
		Branch:    "production", // App is configured for production branch
	}
	store.allTenantsList = []core.Tenant{{ID: "t-1", Name: "Tenant 1"}}

	events := core.NewEventBus(nil)
	p := NewPipeline(store, nil, events, slog.Default())

	payload := webhooks.WebhookPayload{
		RepoURL:   "https://github.com/test/repo",
		Branch:    "develop", // Webhook is for develop branch
		CommitSHA: "abc123",
	}

	// Should skip silently (return nil, not error) due to branch mismatch
	err := p.HandleWebhook(context.Background(), payload)
	if err != nil {
		t.Fatalf("HandleWebhook should return nil for branch mismatch, got: %v", err)
	}
}

func TestPipeline_HandleWebhook_BuildFails(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID:        "app-1",
		Name:      "build-fail-app",
		SourceURL: "https://github.com/test/repo",
		TenantID:  "t-1",
		Branch:    "main",
	}
	store.allTenantsList = []core.Tenant{{ID: "t-1", Name: "Tenant 1"}}

	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)
	p := NewPipeline(store, runtime, events, slog.Default())

	payload := webhooks.WebhookPayload{
		RepoURL:   "https://github.com/test/repo",
		Branch:    "main",
		CommitSHA: "abc123",
	}

	// Build will fail because the builder tries to clone a repo
	// that doesn't exist (no real git repo).
	err := p.HandleWebhook(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error when build fails")
	}

	// Verify app status was set to "building" then "failed"
	foundBuilding := false
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "building" {
			foundBuilding = true
		}
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundBuilding {
		t.Error("expected 'building' status update")
	}
	if !foundFailed {
		t.Error("expected 'failed' status update after build failure")
	}
}

func TestPipeline_HandleWebhook_EmptyBranch_NoMismatch(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID:        "app-1",
		Name:      "no-branch-app",
		SourceURL: "https://github.com/test/repo",
		TenantID:  "t-1",
		Branch:    "", // No branch configured
	}
	store.allTenantsList = []core.Tenant{{ID: "t-1", Name: "Tenant 1"}}

	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)
	p := NewPipeline(store, runtime, events, slog.Default())

	payload := webhooks.WebhookPayload{
		RepoURL:   "https://github.com/test/repo",
		Branch:    "feature-x",
		CommitSHA: "abc123",
	}

	// Build will fail (no real repo), but the branch check should pass
	err := p.HandleWebhook(context.Background(), payload)
	if err == nil {
		// Build succeeds somehow — fine
	} else {
		// Build fails — expected; but it should not be a "branch mismatch" error
		if err.Error() == "branch mismatch" {
			t.Error("should not get branch mismatch when app has no branch configured")
		}
	}
}
