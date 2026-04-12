package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Module — Health returns HealthDegraded when docker is nil
// =============================================================================

func TestModule_Health_NilDocker_Degraded(t *testing.T) {
	m := New()
	if m.Health() != core.HealthDegraded {
		t.Errorf("Health() = %v, want HealthDegraded", m.Health())
	}
}

// =============================================================================
// Module — Stop returns nil when docker is nil
// =============================================================================

func TestModule_Stop_Nil(t *testing.T) {
	m := New()
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}

// =============================================================================
// Module — Init with store sets store field
// =============================================================================

func TestModule_Init_SetsStore(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    store,
		Config:   &core.Config{},
		Services: core.NewServices(),
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.store != store {
		t.Error("store not set after Init")
	}
	// Docker should be nil (no Docker daemon available in tests)
	if m.docker != nil {
		t.Log("Docker is available in test environment (unexpected but OK)")
	}
}

// =============================================================================
// Module — Init with nil store returns error
// =============================================================================

func TestModule_Init_NilStore_ReturnsError(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  nil,
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error when Store is nil")
	}
}

// =============================================================================
// Module — Start with nil docker does not error
// =============================================================================

func TestModule_Start_NilDocker_LogsStarted(t *testing.T) {
	m := New()
	m.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// =============================================================================
// Pipeline — HandleWebhook branch mismatch with empty webhook branch
// =============================================================================

// =============================================================================
// Pipeline — HandleWebhook branches both empty
// =============================================================================

// =============================================================================
// Pipeline — HandleWebhook matching branches
// =============================================================================

// =============================================================================
// Rollback — ListDeployments error
// =============================================================================

func TestRollback_ListDeploymentsError(t *testing.T) {
	store := newMockStore()
	store.listDeploymentsErr = fmt.Errorf("db unavailable")
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-1", 1)
	if err == nil {
		t.Fatal("expected error when ListDeploymentsByApp fails")
	}
}

// =============================================================================
// Rollback — target version not found
// =============================================================================

func TestRollback_TargetVersionNotFound(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "v1", Status: "done"},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-1", 99)
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
}

// =============================================================================
// Rollback — target deployment has no image
// =============================================================================

func TestRollback_EmptyImage(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "", Status: "done"},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-1", 1)
	if err == nil {
		t.Fatal("expected error when deployment has no image")
	}
}

// =============================================================================
// Rollback — GetApp error
// =============================================================================

func TestRollback_GetAppError(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1", Status: "done"},
	}
	store.getAppErr = fmt.Errorf("app not found")
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-1", 1)
	if err == nil {
		t.Fatal("expected error when GetApp fails")
	}
}

// =============================================================================
// Rollback — nil runtime (no container operations)
// =============================================================================

func TestRollback_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1", Status: "stopped"},
	}
	store.apps["app-nr"] = &core.Application{
		ID: "app-nr", Name: "no-runtime-app", TenantID: "t1",
	}
	store.latestDeployment = &core.Deployment{ContainerID: "old-ctr"}
	store.nextVersion = 2
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	dep, err := re.Rollback(context.Background(), "app-nr", 1)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if dep.Status != "running" {
		t.Errorf("Status = %q, want running", dep.Status)
	}
	// ContainerID should be empty since runtime is nil
	if dep.ContainerID != "" {
		t.Errorf("ContainerID = %q, want empty", dep.ContainerID)
	}
}

// =============================================================================
// Rollback — runtime CreateAndStart fails
// =============================================================================

func TestRollback_RuntimeCreateFails(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1", Status: "stopped"},
	}
	store.apps["app-cf"] = &core.Application{
		ID: "app-cf", Name: "create-fail", TenantID: "t1",
	}
	store.latestDeployment = nil // No current deployment
	store.nextVersion = 2

	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("out of memory")
		},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, runtime, events)

	_, err := re.Rollback(context.Background(), "app-cf", 1)
	if err == nil {
		t.Fatal("expected error when CreateAndStart fails")
	}

	// Verify app status was set to failed
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Error("expected 'failed' status when runtime fails")
	}
}

// =============================================================================
// Rollback — with runtime success and existing container
// =============================================================================

func TestRollback_FullSuccess_WithOldContainer(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Image: "app:v2", Status: "running"},
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-full"] = &core.Application{
		ID: "app-full", Name: "full-rollback", TenantID: "t1",
	}
	store.latestDeployment = &core.Deployment{ContainerID: "old-ctr-id"}
	store.nextVersion = 3

	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	var receivedEvent bool
	events.Subscribe(core.EventRollbackDone, func(_ context.Context, ev core.Event) error {
		receivedEvent = true
		return nil
	})

	re := NewRollbackEngine(store, runtime, events)
	dep, err := re.Rollback(context.Background(), "app-full", 1)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if dep.Status != "running" {
		t.Errorf("Status = %q", dep.Status)
	}
	if dep.Image != "app:v1" {
		t.Errorf("Image = %q, want app:v1", dep.Image)
	}
	if dep.Version != 3 {
		t.Errorf("Version = %d, want 3", dep.Version)
	}
	if dep.ContainerID != "container-new-123" {
		t.Errorf("ContainerID = %q", dep.ContainerID)
	}
	if !runtime.stopCalled {
		t.Error("Stop should be called for old container")
	}
	if !runtime.removeCalled {
		t.Error("Remove should be called for old container")
	}
	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
	if !receivedEvent {
		t.Error("EventRollbackDone should be published")
	}

	// Verify rollback label
	if runtime.lastOpts.Labels["monster.rollback.from"] != "1" {
		t.Errorf("rollback.from label = %q, want 1",
			runtime.lastOpts.Labels["monster.rollback.from"])
	}
}

// =============================================================================
// Rollback — no current deployment (GetLatestDeployment returns nil)
// =============================================================================

func TestRollback_NoPreviousDeployment(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-no-prev"] = &core.Application{
		ID: "app-no-prev", Name: "no-prev", TenantID: "t1",
	}
	store.latestDeployment = nil // No current deployment
	store.nextVersion = 2

	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, runtime, events)

	dep, err := re.Rollback(context.Background(), "app-no-prev", 1)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if dep.Status != "running" {
		t.Errorf("Status = %q", dep.Status)
	}
	// Stop/Remove should NOT be called since there's no old container
	if runtime.stopCalled {
		t.Error("Stop should not be called when no old container")
	}
}

// =============================================================================
// Rollback — ListVersions error
// =============================================================================

func TestRollback_ListVersions_Error(t *testing.T) {
	store := newMockStore()
	store.listDeploymentsErr = fmt.Errorf("db read error")
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.ListVersions(context.Background(), "app-1", 10)
	if err == nil {
		t.Fatal("expected error when ListDeploymentsByApp fails")
	}
}

// =============================================================================
// Rollback — ListVersions empty
// =============================================================================

func TestRollback_ListVersions_Empty(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	versions, err := re.ListVersions(context.Background(), "app-1", 10)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0, got %d", len(versions))
	}
}

// =============================================================================
// ImageUpdateChecker — Start and Stop
// =============================================================================


// =============================================================================
// ImageUpdateChecker — checkAll with ListAppsByTenant error
// =============================================================================


// =============================================================================
// ImageUpdateChecker — checkAll with apps of various types
// =============================================================================


// =============================================================================
// AutoRestarter — checkCrashed with nil runtime
// =============================================================================

func TestCov_AutoRestarter_CheckCrashed_NilRuntime(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ar := NewAutoRestarter(nil, store, events, logger)
	// Should return early without error
	ar.checkCrashed()
}

// =============================================================================
// AutoRestarter — checkCrashed with containers in various states
// =============================================================================

func TestAutoRestarter_CheckCrashed_DeadWithAppID(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
				{ID: "c2", State: "dead", Labels: map[string]string{"monster.app.id": "a2"}},
				{ID: "c3", State: "exited", Labels: map[string]string{}}, // no app ID
			}, nil
		},
		restartFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1
	ar.checkCrashed()

	// Only c2 should trigger handleCrash (dead + has app ID)
	// c1 is running, c3 has no app ID
}

// =============================================================================
// AutoRestarter — handleCrash with retry failure then success
// =============================================================================

func TestAutoRestarter_HandleCrash_RetryThenSuccess(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	attempt := 0
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			attempt++
			if attempt < 2 {
				return fmt.Errorf("restart failed attempt %d", attempt)
			}
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 3

	ar.handleCrash(context.Background(), "app-retry", "ctr-retry")

	if attempt < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempt)
	}

	foundRunning := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "running" {
			foundRunning = true
		}
	}
	if !foundRunning {
		t.Error("expected 'running' status after successful retry")
	}
}

// =============================================================================
// AutoRestarter — handleCrash all retries fail
// =============================================================================

func TestAutoRestarter_HandleCrash_AllRetriesFail(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("always fails")
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 2

	ar.handleCrash(context.Background(), "app-allfail", "ctr-allfail")

	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Error("expected 'failed' status after all retries exhausted")
	}
}

// =============================================================================
// Deployer — DeployImage with various version numbers
// =============================================================================

func TestDeployer_DeployImage_HighVersion(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 42
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID: "app-hv", Name: "high-version", TenantID: "t1", ProjectID: "p1",
	}

	dep, err := d.DeployImage(context.Background(), app, "myapp:v42")
	if err != nil {
		t.Fatalf("DeployImage: %v", err)
	}
	if dep.Version != 42 {
		t.Errorf("Version = %d, want 42", dep.Version)
	}
	if dep.AppID != "app-hv" {
		t.Errorf("AppID = %q", dep.AppID)
	}
	if runtime.lastOpts.Labels["monster.deploy.version"] != "42" {
		t.Errorf("version label = %q", runtime.lastOpts.Labels["monster.deploy.version"])
	}
	if runtime.lastOpts.Labels["monster.enable"] != "true" {
		t.Errorf("enable label = %q", runtime.lastOpts.Labels["monster.enable"])
	}
	if runtime.lastOpts.RestartPolicy != "unless-stopped" {
		t.Errorf("RestartPolicy = %q", runtime.lastOpts.RestartPolicy)
	}
	if runtime.lastOpts.Network != "monster-network" {
		t.Errorf("Network = %q", runtime.lastOpts.Network)
	}
}

// =============================================================================
// NewDockerManager — empty host
// =============================================================================

func TestNewDockerManager_EmptyHost(t *testing.T) {
	// Empty host should use default Docker socket
	_, err := NewDockerManager("")
	if err != nil {
		t.Logf("NewDockerManager with empty host failed (expected in CI): %v", err)
	}
}

// =============================================================================
// Pipeline — findAppBySourceURL with multiple tenants
// =============================================================================

// =============================================================================
// VersionInfo struct verification
// =============================================================================

func TestVersionInfo_Struct(t *testing.T) {
	v := VersionInfo{
		Version:   3,
		Image:     "app:v3",
		Status:    "running",
		CommitSHA: "abc123",
		IsCurrent: true,
	}
	if v.Version != 3 {
		t.Errorf("Version = %d", v.Version)
	}
	if !v.IsCurrent {
		t.Error("IsCurrent should be true")
	}
}

// =============================================================================
// Pipeline — HandleWebhook emits DeployStarted and DeployFailed events
// =============================================================================

// =============================================================================
// NewAutoRestarter — field assignment
// =============================================================================

func TestNewAutoRestarter_Fields(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ar := NewAutoRestarter(runtime, store, events, logger)
	if ar.runtime != runtime {
		t.Error("runtime mismatch")
	}
	if ar.store != store {
		t.Error("store mismatch")
	}
	if ar.events != events {
		t.Error("events mismatch")
	}
	if ar.logger != logger {
		t.Error("logger mismatch")
	}
	if ar.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", ar.maxRetries)
	}
}

// =============================================================================
// NewImageUpdateChecker — field assignment
// =============================================================================


// =============================================================================
// EnsureNetwork — Docker-specific, test with mock approach
// =============================================================================

func TestDockerManager_EnsureNetwork_NeedsDocker(t *testing.T) {
	// This tests the code path for EnsureNetwork — requires Docker.
	// In CI/test environments without Docker, NewDockerManager fails,
	// so we test the branch coverage via module.Start instead.
	m := New()
	m.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	// docker is nil — Start with nil docker skips EnsureNetwork
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// =============================================================================
// Module — full lifecycle (Init -> Start -> Stop) with nil docker
// =============================================================================

func TestModule_FullLifecycle_NilDocker(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    store,
		Config:   &core.Config{},
		Services: core.NewServices(),
		Events:   core.NewEventBus(nil),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if m.Docker() != nil {
		t.Log("Docker available in test (unexpected but OK)")
	}
}
