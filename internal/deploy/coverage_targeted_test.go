package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Module.init — covers the init() registration path (module.go:12, 50.0%)
// =============================================================================

func TestModuleInit_Registered(t *testing.T) {
	m := New()
	if m.ID() != "deploy" {
		t.Errorf("ID() = %q, want %q", m.ID(), "deploy")
	}
}

// =============================================================================
// Module.Init — covers error paths (module.go:54)
// =============================================================================

func TestModule_Init_NilStore_Error(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  nil,
		Config: &core.Config{},
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error when Store is nil")
	}
	if !strings.Contains(err.Error(), "store not available") {
		t.Errorf("error = %v, want 'store not available'", err)
	}
}

func TestModule_Init_DockerNotAvailable(t *testing.T) {
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
		t.Fatalf("expected no error (Docker failure is non-fatal), got: %v", err)
	}
	if m.store != store {
		t.Error("store should be set after Init")
	}
}

func TestModule_Init_DockerWithResourceDefaults(t *testing.T) {
	m := New()
	store := newMockStore()
	cfg := &core.Config{}
	cfg.Docker.DefaultCPUQuota = 50000
	cfg.Docker.DefaultMemoryMB = 256
	cfg.Docker.BuildRegistryUsername = ""
	cfg.Docker.BuildRegistryPassword = ""

	c := &core.Core{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    store,
		Config:   cfg,
		Services: core.NewServices(),
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// =============================================================================
// Module.Start — without Docker (module.go:91)
// =============================================================================

func TestModule_Start_NoDocker(t *testing.T) {
	m := New()
	m.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// =============================================================================
// Module.Start — with Docker and store but without autoRestart/autoRollback
// (exercises reclaimStaleDeployments, cleanOrphanContainers)
// =============================================================================

func TestModule_Start_WithDocker(t *testing.T) {
	m := New()
	m.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	m.store = newMockStore()
	m.core = &core.Core{
		Events: core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil))),
	}

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, nil
		},
	}
	m.docker = runtime

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// =============================================================================
// reclaimStaleDeployments — error paths (module.go:136)
// =============================================================================

func TestReclaimStaleDeployments_ListError(t *testing.T) {
	m := &Module{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		store: &mockStore{
			listByStatusErr: fmt.Errorf("db error"),
		},
	}
	m.reclaimStaleDeployments(context.Background())
	// Should not panic; error is logged
}

func TestReclaimStaleDeployments_UpdateError(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.deploymentsByID["dep-1"] = &core.Deployment{
		ID:    "dep-1",
		AppID: "app-1",
		Status: "deploying",
		Version: 1,
		StartedAt: &now,
	}
	store.deployments = []core.Deployment{
		{ID: "dep-1", AppID: "app-1", Status: "deploying", Version: 1, StartedAt: &now},
	}
	store.updateDeploymentErr = fmt.Errorf("update failed")

	m := &Module{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:  store,
	}
	m.reclaimStaleDeployments(context.Background())
	// Should not panic
}

func TestReclaimStaleDeployments_Success(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.deploymentsByID["dep-1"] = &core.Deployment{
		ID:    "dep-1",
		AppID: "app-1",
		Status: "deploying",
		Version: 1,
		StartedAt: &now,
		BuildLog:  "",
	}
	store.deployments = []core.Deployment{
		{ID: "dep-1", AppID: "app-1", Status: "deploying", Version: 1, StartedAt: &now},
	}
	store.apps["app-1"] = &core.Application{
		ID:       "app-1",
		TenantID: "tenant-1",
	}

	m := &Module{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:  store,
	}
	m.reclaimStaleDeployments(context.Background())

	// Verify the deployment was updated
	if store.updateDeploymentCall == 0 {
		t.Error("expected UpdateDeployment to be called")
	}
	dep := store.deploymentsByID["dep-1"]
	if dep == nil || dep.Status != "failed" {
		t.Errorf("deployment status = %q, want 'failed'", dep.Status)
	}
}

// =============================================================================
// SetRegistryAuth — error path (docker.go:62)
// =============================================================================

func TestSetRegistryAuth_OneEmpty_Error(t *testing.T) {
	d := &DockerManager{}
	err := d.SetRegistryAuth("user", "")
	if err == nil || !strings.Contains(err.Error(), "must both be set") {
		t.Errorf("expected both-set error, got: %v", err)
	}
}

func TestSetRegistryAuth_NoAuth_Clears(t *testing.T) {
	d := &DockerManager{}
	d.registryAuth = "old-auth"
	err := d.SetRegistryAuth("", "")
	if err != nil {
		t.Fatalf("SetRegistryAuth: %v", err)
	}
	if d.registryAuth != "" {
		t.Error("registryAuth should be cleared when both are empty")
	}
}

func TestSetRegistryAuth_ValidCredentials(t *testing.T) {
	d := &DockerManager{}
	err := d.SetRegistryAuth("user", "pass")
	if err != nil {
		t.Fatalf("SetRegistryAuth: %v", err)
	}
	if d.registryAuth == "" {
		t.Error("registryAuth should be set after valid credentials")
	}
}

// =============================================================================
// AutoRestarter — Start with nil events (autorestart.go:54)
// =============================================================================

func TestAutoRestarter_Start_NilEvents(t *testing.T) {
	ar := NewAutoRestarter(&mockRuntime{}, newMockStore(), nil, nil)
	ar.Start()
	ar.Stop()
	// Should not panic
}

func TestAutoRestarter_Stop_Multiple(t *testing.T) {
	ar := NewAutoRestarter(&mockRuntime{}, newMockStore(), core.NewEventBus(slog.Default()), nil)
	ar.Stop()
	ar.Stop() // Second call should be a no-op
}

// =============================================================================
// AutoRollbackManager — Start with nil events (autorollback.go:83)
// =============================================================================

func TestAutoRollbackManager_Start_NilEvents(t *testing.T) {
	arm := NewAutoRollbackManager(newMockStore(), &mockRuntime{}, nil, nil)
	arm.Start()
	arm.Stop()
	// Should not panic
}

func TestAutoRollbackManager_Stop_Multiple(t *testing.T) {
	arm := NewAutoRollbackManager(newMockStore(), &mockRuntime{}, core.NewEventBus(slog.Default()), nil)
	arm.Stop()
	arm.Stop() // Second call should be a no-op
}

// =============================================================================
// AutoRollbackManager — handleFailure cooldown (autorollback.go:178)
// =============================================================================

func TestAutoRollbackManager_HandleFailure_Cooldown(t *testing.T) {
	arm := NewAutoRollbackManager(newMockStore(), &mockRuntime{}, core.NewEventBus(slog.Default()), nil)
	arm.lastAttempt["app-1"] = time.Now() // Just now, so cooldown is active

	// Should log and return without error
	arm.handleFailure(context.Background(), "app-1")
	// No panic is the success condition
}

func TestAutoRollbackManager_HandleFailure_NoStableVersion(t *testing.T) {
	store := newMockStore()
	arm := NewAutoRollbackManager(store, &mockRuntime{}, core.NewEventBus(slog.Default()), nil)

	// No deployments means no stable version found
	arm.handleFailure(context.Background(), "app-1")
	// Should log warning and return
}

// =============================================================================
// AutoRollbackManager — isClosed / runCtx / Wait (autorollback.go)
// =============================================================================

func TestAutoRollbackManager_IsClosed(t *testing.T) {
	arm := NewAutoRollbackManager(newMockStore(), &mockRuntime{}, nil, nil)
	if arm.isClosed() {
		t.Error("isClosed should be false before Stop")
	}
	arm.Stop()
	if !arm.isClosed() {
		t.Error("isClosed should be true after Stop")
	}
}

func TestAutoRollbackManager_RunCtx_FallsBackToEventCtx(t *testing.T) {
	arm := &AutoRollbackManager{}
	eventCtx := context.WithValue(context.Background(), "key", "value")
	ctx := arm.runCtx(eventCtx)
	if ctx != eventCtx {
		t.Error("runCtx should return eventCtx when stopCtx is nil")
	}
}

func TestAutoRollbackManager_RunCtx_FallsBackToBackground(t *testing.T) {
	arm := &AutoRollbackManager{}
	ctx := arm.runCtx(nil)
	if ctx != context.Background() {
		t.Error("runCtx should return Background when both are nil")
	}
}

// =============================================================================
// AutoRollbackManager — handleFailure with closed flag (autorollback.go:178)
// =============================================================================

func TestAutoRollbackManager_HandleFailure_Closed(t *testing.T) {
	arm := NewAutoRollbackManager(newMockStore(), &mockRuntime{}, nil, nil)
	arm.closed = true
	arm.handleFailure(context.Background(), "app-1")
	// Should return immediately
}

// =============================================================================
// handleCrash — runtime nil path (autorestart.go:90)
// =============================================================================

// =============================================================================
// Rollback — error path (rollback.go:25)
// =============================================================================

func TestRollback_UpdateDeploymentStatusError(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "test-app", TenantID: "tenant-1"}
	now := time.Now()
	store.deployments = []core.Deployment{
		{ID: "dep-1", AppID: "app-1", Version: 1, Status: "running", Image: "nginx:latest", StartedAt: &now},
	}
	store.updateDeploymentErr = fmt.Errorf("persist error")

	engine := NewRollbackEngine(store, &mockRuntime{}, nil)

	_, err := engine.Rollback(context.Background(), "app-1", 1)
	if err == nil || !strings.Contains(err.Error(), "persist rollback") {
		t.Errorf("expected persist error, got: %v", err)
	}
}

func TestRollback_WithRuntime(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "test-app", TenantID: "tenant-1"}
	now := time.Now()
	store.deployments = []core.Deployment{
		{ID: "dep-1", AppID: "app-1", Version: 1, Status: "running", Image: "nginx:latest", StartedAt: &now},
	}
	store.deploymentsByID["dep-1"] = &core.Deployment{
		ID: "dep-1", AppID: "app-1", Version: 1, Status: "running", Image: "nginx:latest", StartedAt: &now,
	}

	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "new-container-id", nil
		},
	}
	engine := NewRollbackEngine(store, runtime, nil)

	dep, err := engine.Rollback(context.Background(), "app-1", 1)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if dep == nil {
		t.Fatal("expected deployment, got nil")
	}
	if dep.Image != "nginx:latest" {
		t.Errorf("image = %q, want nginx:latest", dep.Image)
	}
}

// =============================================================================
// AutoRestarter — NewAutoRestarter with nil logger (autorestart.go:38)
// =============================================================================

func TestNewAutoRestarter_NilLogger(t *testing.T) {
	ar := NewAutoRestarter(nil, nil, nil, nil)
	if ar == nil {
		t.Fatal("NewAutoRestarter returned nil")
	}
	if ar.logger == nil {
		t.Error("logger should not be nil after construction with nil arg")
	}
}

func TestNewAutoRestarter_DefaultRetryDelay(t *testing.T) {
	delay := defaultAutoRestartRetryDelay(3)
	if delay != 15*time.Second {
		t.Errorf("delay = %v, want 15s", delay)
	}
}

// =============================================================================
// AutoRollbackManager — findLastStable (autorollback.go:232)
// =============================================================================

func TestAutoRollbackManager_FindLastStable_Empty(t *testing.T) {
	store := newMockStore()
	arm := NewAutoRollbackManager(store, nil, nil, nil)

	_, err := arm.findLastStable(context.Background(), "app-1")
	if err == nil {
		t.Fatal("expected error for empty deployments")
	}
}
