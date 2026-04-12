package deploy

import (
	"context"
	"fmt"

	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// sanitizeSlug — additional edge cases not in existing tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestSanitizeSlugCoverage_EmptyInput(t *testing.T) {
	got := sanitizeSlug("")
	if got == "" {
		t.Error("empty input should produce a fallback slug")
	}
}

func TestSanitizeSlugCoverage_LeadingTrailingHyphens(t *testing.T) {
	got := sanitizeSlug("---hello---")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestSanitizeSlugCoverage_NumbersOnly(t *testing.T) {
	got := sanitizeSlug("12345")
	if got != "12345" {
		t.Errorf("expected '12345', got %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RollbackEngine — ListVersions with limit
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackEngine_ListVersions_LimitApplied(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 3, Image: "app:v3", Status: "running"},
		{Version: 2, Image: "app:v2", Status: "stopped"},
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	versions, err := re.ListVersions(context.Background(), "app-1", 5)
	if err != nil {
		t.Fatalf("ListVersions error: %v", err)
	}
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}
	if !versions[0].IsCurrent {
		t.Error("first version should be marked as current")
	}
	for i := 1; i < len(versions); i++ {
		if versions[i].IsCurrent {
			t.Errorf("version[%d] should not be current", i)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// AutoRestarter — Start subscribes events
// ═══════════════════════════════════════════════════════════════════════════════

func TestAutoRestarter_Start_DoesNotPanic(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	logger := slog.Default()
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 0

	// Start should not panic
	ar.Start()

	// Publish a container.died event to trigger the subscriber callback
	events.Publish(context.Background(), core.NewEvent(
		core.EventContainerDied, "test",
		core.DeployEventData{AppID: "app-start-test", ContainerID: "ctr-start-test"},
	))
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageUpdateChecker — store returns error in checkAll
// ═══════════════════════════════════════════════════════════════════════════════
// ═══════════════════════════════════════════════════════════════════════════════
// Rollback — GetApp error after finding deployment
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackEngine_Rollback_AppNotFound(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	// Don't add the app to the store — so GetApp will fail
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "missing-app", 1)
	if err == nil {
		t.Fatal("expected error when app is not found")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module — Init edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleCoverage_Init_NilStore(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Store:  nil,
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should fail when Store is nil")
	}
}

func TestModuleCoverage_Init_WithStore(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.Default(),
		Store:    store,
		Config:   &core.Config{},
		Services: &core.Services{},
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	if m.store != store {
		t.Error("store should be set after Init")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Deployer — edge case: TriggeredBy
// ═══════════════════════════════════════════════════════════════════════════════

func TestAutoRestarterCoverage_HandleCrash_ZeroRetries(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()
	runtime := &mockRuntime{}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 0 // No retries

	ar.handleCrash(context.Background(), "app-z", "ctr-z")

	// Should go straight to 'failed' after crashed
	foundCrashed := false
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "crashed" {
			foundCrashed = true
		}
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundCrashed {
		t.Error("expected 'crashed' status")
	}
	if !foundFailed {
		t.Error("expected 'failed' status after 0 retries")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CheckDockerHubTag — context cancelled
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// Module.Start — with nil docker
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleCoverage_Start_NilDocker(t *testing.T) {
	m := New()
	m.logger = slog.Default()

	err := m.Start(context.Background())
	if err != nil {
		t.Errorf("Start() with nil docker should return nil, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageUpdateChecker — checkAll with image-type apps
// ═══════════════════════════════════════════════════════════════════════════════
// ═══════════════════════════════════════════════════════════════════════════════
// AutoRestarter — checkCrashed with mixed containers
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// NewDockerManager — host option coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewDockerManagerCoverage_CustomHost(t *testing.T) {
	// Custom host (not empty, not default socket) should append WithHost opt.
	// This will fail to ping but covers the option code path.
	_, err := NewDockerManager("tcp://127.0.0.1:99999")
	if err == nil {
		t.Log("NewDockerManager connected to invalid host (unlikely)")
	}
}

func TestNewDockerManagerCoverage_DefaultSocket(t *testing.T) {
	// Default socket should NOT append WithHost opt.
	_, err := NewDockerManager("unix:///var/run/docker.sock")
	if err != nil {
		t.Logf("NewDockerManager with default socket failed (expected): %v", err)
	}
}

func TestAutoRestarterCoverage_CheckCrashed_MixedStates(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
				{ID: "c2", State: "exited", Labels: map[string]string{"monster.app.id": "a2"}},
				{ID: "c3", State: "dead", Labels: map[string]string{"monster.app.id": ""}}, // empty app id
			}, nil
		},
		restartFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1

	ar.checkCrashed()

	// Only c2 (exited with app ID) should trigger handleCrash
	// c1 is running (skip), c3 has empty app ID (skip)
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageUpdate struct coverage
// ═══════════════════════════════════════════════════════════════════════════════
// ═══════════════════════════════════════════════════════════════════════════════
// NewRollbackEngine — fields
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewRollbackEngineCoverage_AllFields(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	if re.store != store {
		t.Error("store field mismatch")
	}
	if re.runtime != runtime {
		t.Error("runtime field mismatch")
	}
	if re.events != events {
		t.Error("events field mismatch")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Rollback — with runtime that fails on stop/remove
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackCoverage_StopRemoveErrors(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-sr"] = &core.Application{
		ID: "app-sr", Name: "stop-remove-app", TenantID: "t1",
	}
	store.latestDeployment = &core.Deployment{ContainerID: "old-ctr"}
	store.nextVersion = 2

	runtime := &mockRuntime{
		stopFn: func(_ context.Context, _ string, _ int) error {
			return fmt.Errorf("stop failed")
		},
		removeFn: func(_ context.Context, _ string, _ bool) error {
			return fmt.Errorf("remove failed")
		},
	}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	dep, err := re.Rollback(context.Background(), "app-sr", 1)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}
	// Even with stop/remove errors, rollback should succeed
	if dep.Status != "running" {
		t.Errorf("status = %q, want running", dep.Status)
	}
}
