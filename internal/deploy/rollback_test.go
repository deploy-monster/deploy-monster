package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestNewRollbackEngine(t *testing.T) {
	events := core.NewEventBus(nil)

	t.Run("with nil dependencies", func(t *testing.T) {
		re := NewRollbackEngine(nil, nil, nil)
		if re == nil {
			t.Fatal("NewRollbackEngine returned nil")
		}
	})

	t.Run("with event bus", func(t *testing.T) {
		re := NewRollbackEngine(nil, nil, events)
		if re == nil {
			t.Fatal("NewRollbackEngine returned nil")
		}
	})

	t.Run("fields are set", func(t *testing.T) {
		store := newMockStore()
		re := NewRollbackEngine(store, nil, events)
		if re == nil {
			t.Fatal("NewRollbackEngine returned nil")
		}
		if re.store != store {
			t.Error("store field not set correctly")
		}
		if re.events != events {
			t.Error("events field not set correctly")
		}
		if re.runtime != nil {
			t.Error("runtime should be nil when passed nil")
		}
	})
}

func TestRollbackEngine_ListVersions_NoDeployments(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	versions, err := re.ListVersions(context.Background(), "app-123", 10)
	if err != nil {
		t.Fatalf("ListVersions returned error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected empty versions list, got %d", len(versions))
	}
}

func TestRollbackEngine_ListVersions_WithDeployments(t *testing.T) {
	now := time.Now()
	store := newMockStore()
	store.deployments = []core.Deployment{
		{
			ID:        "dep-1",
			AppID:     "app-123",
			Version:   3,
			Image:     "nginx:1.25",
			Status:    "running",
			CommitSHA: "abc123",
			CreatedAt: now,
		},
		{
			ID:        "dep-2",
			AppID:     "app-123",
			Version:   2,
			Image:     "nginx:1.24",
			Status:    "stopped",
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "dep-3",
			AppID:     "app-123",
			Version:   1,
			Image:     "nginx:1.23",
			Status:    "stopped",
			CreatedAt: now.Add(-2 * time.Hour),
		},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	versions, err := re.ListVersions(context.Background(), "app-123", 10)
	if err != nil {
		t.Fatalf("ListVersions returned error: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}

	// First entry should be marked as current
	if !versions[0].IsCurrent {
		t.Error("first version should be marked as current")
	}
	if versions[1].IsCurrent {
		t.Error("second version should not be marked as current")
	}
	if versions[2].IsCurrent {
		t.Error("third version should not be marked as current")
	}

	// Check mapping correctness
	if versions[0].Version != 3 {
		t.Errorf("expected version 3, got %d", versions[0].Version)
	}
	if versions[0].Image != "nginx:1.25" {
		t.Errorf("expected image nginx:1.25, got %s", versions[0].Image)
	}
	if versions[0].Status != "running" {
		t.Errorf("expected status running, got %s", versions[0].Status)
	}
	if versions[0].CommitSHA != "abc123" {
		t.Errorf("expected commit sha abc123, got %s", versions[0].CommitSHA)
	}
}

func TestRollbackEngine_ListVersions_StoreError(t *testing.T) {
	store := newMockStore()
	store.listDeploymentsErr = fmt.Errorf("database connection lost")
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	versions, err := re.ListVersions(context.Background(), "app-123", 10)
	if err == nil {
		t.Fatal("expected error from ListVersions when store fails")
	}
	if versions != nil {
		t.Errorf("expected nil versions on error, got %v", versions)
	}
}

func TestRollbackEngine_Rollback_VersionNotFound(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-123", 99)
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
	if err.Error() != "deployment version 99 not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestRollbackEngine_Rollback_EmptyImage(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "", Status: "failed"},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-123", 1)
	if err == nil {
		t.Fatal("expected error when target version has no image")
	}
	if err.Error() != "version 1 has no image to rollback to" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestRollbackEngine_Rollback_ListError(t *testing.T) {
	store := newMockStore()
	store.listDeploymentsErr = fmt.Errorf("db error")
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-123", 1)
	if err == nil {
		t.Fatal("expected error when store.ListDeploymentsByApp fails")
	}
}

func TestRollbackEngine_Rollback_Success_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Image: "nginx:1.25", Status: "running"},
		{Version: 1, Image: "nginx:1.24", Status: "stopped"},
	}
	store.apps["app-123"] = &core.Application{
		ID:       "app-123",
		Name:     "test-app",
		TenantID: "tenant-1",
	}
	store.latestDeployment = &core.Deployment{
		ContainerID: "old-container-id",
	}
	events := core.NewEventBus(nil)

	// With nil runtime, rollback should succeed but skip container operations
	re := NewRollbackEngine(store, nil, events)
	dep, err := re.Rollback(context.Background(), "app-123", 1)
	if err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}
	if dep == nil {
		t.Fatal("expected non-nil deployment")
	}
	if dep.Status != "running" {
		t.Errorf("expected status running, got %s", dep.Status)
	}
	if dep.Image != "nginx:1.24" {
		t.Errorf("expected image nginx:1.24, got %s", dep.Image)
	}
	if dep.CommitMessage != "Rollback to v1" {
		t.Errorf("expected rollback commit message, got %s", dep.CommitMessage)
	}
	if dep.TriggeredBy != "rollback" {
		t.Errorf("expected triggered_by=rollback, got %s", dep.TriggeredBy)
	}
}

func TestRollbackEngine_Rollback_Success_WithRuntime(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 3, Image: "nginx:1.25", Status: "running"},
		{Version: 2, Image: "nginx:1.24", Status: "stopped"},
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	store.apps["app-123"] = &core.Application{
		ID:       "app-123",
		Name:     "rollback-app",
		TenantID: "tenant-1",
	}
	store.latestDeployment = &core.Deployment{
		ContainerID: "old-container-id",
	}
	store.nextVersion = 4

	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	dep, err := re.Rollback(context.Background(), "app-123", 2)
	if err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}

	if !runtime.stopCalled {
		t.Error("old container should be stopped")
	}
	if !runtime.removeCalled {
		t.Error("old container should be removed")
	}
	if !runtime.createCalled {
		t.Error("new container should be created")
	}
	if dep.ContainerID != "container-new-123" {
		t.Errorf("ContainerID = %q, want %q", dep.ContainerID, "container-new-123")
	}
	if dep.Image != "nginx:1.24" {
		t.Errorf("Image = %q, want %q", dep.Image, "nginx:1.24")
	}

	// Verify labels
	opts := runtime.lastOpts
	if opts.Labels["monster.rollback.from"] != "2" {
		t.Errorf("missing rollback.from label, got %q", opts.Labels["monster.rollback.from"])
	}
}

func TestRollbackEngine_Rollback_CreateFails(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	store.apps["app-123"] = &core.Application{
		ID:       "app-123",
		Name:     "fail-app",
		TenantID: "tenant-1",
	}

	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("image not found")
		},
	}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	_, err := re.Rollback(context.Background(), "app-123", 1)
	if err == nil {
		t.Fatal("expected error when CreateAndStart fails")
	}

	// Verify app status was set to "failed"
	found := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "failed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("app status should be set to 'failed' when rollback deploy fails")
	}
}

func TestRollbackEngine_Rollback_GetAppError(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	store.getAppErr = fmt.Errorf("app db error")

	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "nonexistent-app", 1)
	if err == nil {
		t.Fatal("expected error when GetApp fails")
	}
}

func TestVersionInfo_Fields(t *testing.T) {
	now := time.Now()
	v := VersionInfo{
		Version:   5,
		Image:     "myapp:v2.0",
		Status:    "running",
		CommitSHA: "deadbeef",
		CreatedAt: now,
		IsCurrent: true,
	}

	if v.Version != 5 {
		t.Errorf("Version = %d, want 5", v.Version)
	}
	if v.Image != "myapp:v2.0" {
		t.Errorf("Image = %s, want myapp:v2.0", v.Image)
	}
	if v.Status != "running" {
		t.Errorf("Status = %s, want running", v.Status)
	}
	if v.CommitSHA != "deadbeef" {
		t.Errorf("CommitSHA = %s, want deadbeef", v.CommitSHA)
	}
	if !v.IsCurrent {
		t.Error("IsCurrent should be true")
	}
	if !v.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt mismatch")
	}
}

// RACE-002 regression: Rollback must allocate the new version through
// AtomicNextDeployVersion, not the legacy non-atomic GetNextDeployVersion.
// Concurrent rollbacks on the same app would otherwise collide on version
// numbers and produce duplicate deployment rows.
func TestRollbackEngine_Rollback_UsesAtomicNextDeployVersion(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Image: "nginx:1.25", Status: "running"},
		{Version: 1, Image: "nginx:1.24", Status: "stopped"},
	}
	store.apps["app-123"] = &core.Application{ID: "app-123", Name: "a", TenantID: "t"}
	store.nextVersion = 3

	re := NewRollbackEngine(store, nil, core.NewEventBus(nil))
	if _, err := re.Rollback(context.Background(), "app-123", 1); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if store.atomicCalls != 1 {
		t.Errorf("expected 1 atomic call, got %d", store.atomicCalls)
	}
	if store.nonAtomicCalls != 0 {
		t.Errorf("expected 0 non-atomic calls, got %d — caller is still on the legacy path", store.nonAtomicCalls)
	}
}

// RACE-002 regression: When AtomicNextDeployVersion returns an error, the
// rollback must surface it rather than silently using a zero version (the
// pre-fix bug discarded the error and would create a v0 deployment row).
func TestRollbackEngine_Rollback_PropagatesVersionAllocError(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Image: "nginx:1.25", Status: "running"},
		{Version: 1, Image: "nginx:1.24", Status: "stopped"},
	}
	store.apps["app-123"] = &core.Application{ID: "app-123", Name: "a", TenantID: "t"}
	store.nextVersionErr = fmt.Errorf("db offline")

	re := NewRollbackEngine(store, nil, core.NewEventBus(nil))
	_, err := re.Rollback(context.Background(), "app-123", 1)
	if err == nil {
		t.Fatal("expected error when version allocation fails")
	}
}
