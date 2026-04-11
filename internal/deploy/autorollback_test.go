package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func newTestAutoRollback(store *mockStore) *AutoRollbackManager {
	bus := core.NewEventBus(slog.Default())
	return NewAutoRollbackManager(store, &mockRuntime{}, bus, slog.Default())
}

// ─── findLastStable ─────────────────────────────────────────────────────────

func TestFindLastStable_Success(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 5, Status: "failed", Image: "app:v5"},  // index 0 — the failed one
		{Version: 4, Status: "running", Image: "app:v4"}, // index 1 — stable ✓
		{Version: 3, Status: "running", Image: "app:v3"}, // index 2
	}
	ar := newTestAutoRollback(store)

	v, err := ar.findLastStable(context.Background(), "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 4 {
		t.Errorf("expected version 4, got %d", v)
	}
}

func TestFindLastStable_SkipsEmptyImage(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 3, Status: "failed", Image: "app:v3"},
		{Version: 2, Status: "running", Image: ""},       // running but no image
		{Version: 1, Status: "running", Image: "app:v1"}, // first real stable
	}
	ar := newTestAutoRollback(store)

	v, err := ar.findLastStable(context.Background(), "app-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version 1, got %d", v)
	}
}

func TestFindLastStable_NoStableVersion(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "failed", Image: "app:v1"},
	}
	ar := newTestAutoRollback(store)

	_, err := ar.findLastStable(context.Background(), "app-1")
	if err == nil {
		t.Fatal("expected error for no stable version")
	}
}

func TestFindLastStable_OnlyOneDeployment(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Status: "failed", Image: "app:v1"},
	}
	ar := newTestAutoRollback(store)

	_, err := ar.findLastStable(context.Background(), "app-1")
	if err == nil {
		t.Fatal("expected error when only one deployment exists")
	}
}

func TestFindLastStable_StoreError(t *testing.T) {
	store := newMockStore()
	store.listDeploymentsErr = fmt.Errorf("db down")
	ar := newTestAutoRollback(store)

	_, err := ar.findLastStable(context.Background(), "app-1")
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestFindLastStable_EmptyDeployments(t *testing.T) {
	store := newMockStore()
	store.deployments = nil
	ar := newTestAutoRollback(store)

	_, err := ar.findLastStable(context.Background(), "app-1")
	if err == nil {
		t.Fatal("expected error for empty deployment list")
	}
}

// ─── handleFailure ──────────────────────────────────────────────────────────

func TestHandleFailure_SuccessfulRollback(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID:       "app-1",
		Name:     "myapp",
		TenantID: "t1",
		Port:     8080,
	}
	store.deployments = []core.Deployment{
		{Version: 3, Status: "failed", Image: "app:v3"},
		{Version: 2, Status: "running", Image: "app:v2"},
	}
	store.nextVersion = 4

	ar := newTestAutoRollback(store)
	ar.handleFailure(context.Background(), "app-1")

	// Should have updated app status at least once (deploying → running)
	if len(store.appStatusUpdates) == 0 {
		t.Fatal("expected app status updates from rollback")
	}
}

func TestHandleFailure_CooldownSkips(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.apps["app-1"] = &core.Application{
		ID:   "app-1",
		Name: "myapp",
		Port: 80,
	}
	store.nextVersion = 3

	ar := newTestAutoRollback(store)

	// First call — should proceed
	ar.handleFailure(context.Background(), "app-1")
	firstUpdates := len(store.appStatusUpdates)
	if firstUpdates == 0 {
		t.Fatal("first call should trigger rollback")
	}

	// Second call immediately — should be skipped (cooldown)
	ar.handleFailure(context.Background(), "app-1")
	if len(store.appStatusUpdates) != firstUpdates {
		t.Error("second call within cooldown should be skipped")
	}
}

func TestHandleFailure_DifferentAppsNotCooldownBlocked(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.apps["app-2"] = &core.Application{ID: "app-2", Name: "a2", Port: 80}
	store.nextVersion = 3

	ar := newTestAutoRollback(store)

	ar.handleFailure(context.Background(), "app-1")
	first := len(store.appStatusUpdates)

	ar.handleFailure(context.Background(), "app-2")
	if len(store.appStatusUpdates) == first {
		t.Error("different app should not be blocked by first app's cooldown")
	}
}

func TestHandleFailure_NoStableVersion(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Status: "failed", Image: "app:v1"},
	}
	ar := newTestAutoRollback(store)

	// Should not panic and should not trigger any status updates
	ar.handleFailure(context.Background(), "app-1")
	if len(store.appStatusUpdates) != 0 {
		t.Error("expected no status updates when no stable version")
	}
}

// ─── Start ──────────────────────────────────────────────────────────────────

func TestAutoRollbackManager_Start(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "myapp", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	bus := core.NewEventBus(slog.Default())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, slog.Default())
	ar.Start()

	// Publish a deploy.failed event — should trigger handleFailure via async subscription
	bus.Publish(context.Background(), core.Event{
		Type:   core.EventDeployFailed,
		Source: "test",
		Data:   core.DeployEventData{AppID: "app-1"},
	})

	// Give async handler time to run
	time.Sleep(100 * time.Millisecond)

	if len(store.appStatusUpdates) == 0 {
		t.Error("expected rollback to trigger via event subscription")
	}
}

func TestAutoRollbackManager_Start_BadEventData(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(slog.Default())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, slog.Default())
	ar.Start()

	// Publish with wrong data type — should not panic
	bus.Publish(context.Background(), core.Event{
		Type:   core.EventDeployFailed,
		Source: "test",
		Data:   "not a DeployEventData",
	})

	time.Sleep(50 * time.Millisecond)

	if len(store.appStatusUpdates) != 0 {
		t.Error("bad event data should not trigger rollback")
	}
}

// ─── Constructor ────────────────────────────────────────────────────────────

func TestNewAutoRollbackManager(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(slog.Default())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, slog.Default())

	if ar.cooldown != 5*time.Minute {
		t.Errorf("expected 5m cooldown, got %v", ar.cooldown)
	}
	if ar.lastAttempt == nil {
		t.Error("lastAttempt map not initialized")
	}
}
