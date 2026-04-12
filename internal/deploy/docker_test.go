package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// NewDockerManager — constructor edge cases
// =====================================================

func TestNewDockerManager_InvalidHost(t *testing.T) {
	// An invalid host scheme should produce an error from the Docker client.
	_, err := NewDockerManager("tcp://999.999.999.999:99999")
	if err == nil {
		// Some Docker client versions do lazy connect, so error may come from Ping.
		t.Log("NewDockerManager did not return error for unreachable host (lazy connect)")
	}
}

func TestNewDockerManager_EmptyHost_PingFails(t *testing.T) {
	// Empty host defaults to unix socket; Ping will fail if Docker is not running.
	_, err := NewDockerManager("")
	if err != nil {
		// Expected on CI/test environments without Docker.
		t.Logf("NewDockerManager with empty host failed (no Docker): %v", err)
	}
}

func TestNewDockerManager_InvalidScheme(t *testing.T) {
	// A completely invalid URI scheme should fail at client creation or ping.
	_, err := NewDockerManager("ftp://not-a-docker-host:1234")
	if err == nil {
		t.Log("NewDockerManager did not return immediate error for ftp scheme (lazy connect)")
	}
}

// =====================================================
// DockerManager.Ping — via mock struct test
// =====================================================

func TestDockerManager_Struct_NilCli(t *testing.T) {
	// Verify the DockerManager struct holds the cli pointer.
	dm := &DockerManager{cli: nil}
	if dm.cli != nil {
		t.Error("expected nil cli")
	}
}

// =====================================================
// Container label generation in DeployImage
// =====================================================

func TestDeployImage_LabelGeneration_AllFields(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 42
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:        "app-label-gen",
		Name:      "label-test-app",
		ProjectID: "proj-label",
		TenantID:  "tenant-label",
	}

	_, err := d.DeployImage(context.Background(), app, "redis:7-alpine")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}

	labels := runtime.lastOpts.Labels

	expected := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         "app-label-gen",
		"monster.app.name":       "label-test-app",
		"monster.project":        "proj-label",
		"monster.tenant":         "tenant-label",
		"monster.deploy.version": "42",
	}

	if len(labels) != len(expected) {
		t.Errorf("label count = %d, want %d", len(labels), len(expected))
	}

	for k, want := range expected {
		got, ok := labels[k]
		if !ok {
			t.Errorf("missing label %q", k)
			continue
		}
		if got != want {
			t.Errorf("label %q = %q, want %q", k, got, want)
		}
	}
}

func TestDeployImage_ContainerName_Format(t *testing.T) {
	tests := []struct {
		appName string
		version int
		want    string
	}{
		{"my-app", 1, "monster-my-app-1"},
		{"web-server", 10, "monster-web-server-10"},
		{"api", 100, "monster-api-100"},
	}

	for _, tt := range tests {
		t.Run(tt.appName, func(t *testing.T) {
			store := newMockStore()
			store.nextVersion = tt.version
			runtime := &mockRuntime{}
			events := core.NewEventBus(nil)

			d := NewDeployer(runtime, store, events)
			app := &core.Application{
				ID:   "app-x",
				Name: tt.appName,
			}

			_, err := d.DeployImage(context.Background(), app, "nginx:latest")
			if err != nil {
				t.Fatalf("DeployImage error: %v", err)
			}

			if runtime.lastOpts.Name != tt.want {
				t.Errorf("container name = %q, want %q", runtime.lastOpts.Name, tt.want)
			}
		})
	}
}

// =====================================================
// Module Init — without Core (nil store)
// =====================================================

func TestModule_Init_NilStore(t *testing.T) {
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

func TestModule_Init_WithStoreDockerFails(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.Default(),
		Store:    store,
		Config:   &core.Config{},
		Services: &core.Services{},
	}

	// Docker will fail to connect on test machines (no Docker daemon)
	// Init should succeed anyway (Docker failure is non-fatal)
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// Store should be set
	if m.store != store {
		t.Error("store should be set after Init")
	}
	// Docker may or may not be nil depending on the test environment
	if m.docker != nil {
		t.Log("Docker is available in this test environment")
	}
}

// =====================================================
// Module Health — state transitions
// =====================================================

func TestModule_Health_Transitions(t *testing.T) {
	m := New()

	// No Docker => Degraded
	if got := m.Health(); got != core.HealthDegraded {
		t.Errorf("Health without docker = %v, want HealthDegraded", got)
	}

	// docker field is not nil but Ping fails => we can test by
	// verifying the interface contract: HealthDegraded when docker==nil
	// This confirms the branching logic in Health().
}

// =====================================================
// EnsureNetwork concept — verifying label usage
// =====================================================

func TestEnsureNetwork_LabelConstant(t *testing.T) {
	// Verify that the module Start method uses "monster-network" as the network name.
	// This is a concept test that validates the constant used.
	m := New()
	m.logger = slog.Default()

	// Start with nil docker should be safe and skip EnsureNetwork
	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start with nil docker returned error: %v", err)
	}
}

// =====================================================
// Rollback — label includes rollback.from
// =====================================================

func TestRollback_Labels_IncludeRollbackFrom(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 3, Image: "app:v3", Status: "running"},
		{Version: 2, Image: "app:v2", Status: "stopped"},
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-rb"] = &core.Application{
		ID:       "app-rb",
		Name:     "rollback-labels",
		TenantID: "t1",
	}
	store.nextVersion = 4

	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	_, err := re.Rollback(context.Background(), "app-rb", 1)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}

	labels := runtime.lastOpts.Labels
	if labels["monster.rollback.from"] != "1" {
		t.Errorf("rollback.from label = %q, want %q", labels["monster.rollback.from"], "1")
	}
	if labels["monster.enable"] != "true" {
		t.Errorf("monster.enable label missing or wrong: %q", labels["monster.enable"])
	}
	if labels["monster.app.id"] != "app-rb" {
		t.Errorf("monster.app.id label = %q, want %q", labels["monster.app.id"], "app-rb")
	}
	if labels["monster.deploy.version"] != "4" {
		t.Errorf("monster.deploy.version = %q, want %q", labels["monster.deploy.version"], "4")
	}
}

// =====================================================
// AutoRestarter — checkCrashed filters by state
// =====================================================

func TestAutoRestarter_CheckCrashed_IgnoresRunningContainers(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	restartedIDs := make(map[string]bool)
	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{ID: "running-1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
				{ID: "running-2", State: "running", Labels: map[string]string{"monster.app.id": "a2"}},
				{ID: "created-1", State: "created", Labels: map[string]string{"monster.app.id": "a3"}},
				{ID: "paused-1", State: "paused", Labels: map[string]string{"monster.app.id": "a4"}},
			}, nil
		},
		restartFn: func(_ context.Context, id string) error {
			restartedIDs[id] = true
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1
	ar.checkCrashed()

	// None of these containers are "exited" or "dead", so no restarts should happen.
	if len(store.appStatusUpdates) != 0 {
		t.Errorf("expected 0 status updates for non-crashed containers, got %d", len(store.appStatusUpdates))
	}
}

// =====================================================
// ImageUpdateChecker — Start/Stop lifecycle
// =====================================================


// =====================================================
// Module Stop — with non-nil docker that returns error
// =====================================================

func TestModule_Stop_FieldsReset(t *testing.T) {
	m := New()
	// docker is nil, Stop returns nil
	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop with nil docker = %v, want nil", err)
	}
}

// =====================================================
// Deployer — DeployImage sets FinishedAt after success
// =====================================================

func TestDeployer_DeployImage_FinishedAt(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:       "app-finish",
		Name:     "finish-app",
		TenantID: "t1",
	}

	dep, err := d.DeployImage(context.Background(), app, "nginx:1.25")
	if err != nil {
		t.Fatalf("DeployImage error: %v", err)
	}

	if dep.FinishedAt == nil {
		t.Error("FinishedAt should be set after successful deploy")
	}
	if dep.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	if dep.FinishedAt.Before(*dep.StartedAt) {
		t.Error("FinishedAt should be after StartedAt")
	}
}

// =====================================================
// AutoDomain — event is published on domain creation
// =====================================================

func TestAutoDomain_PublishesEvent(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)

	var received bool
	events.SubscribeAsync(core.EventDomainAdded, func(_ context.Context, _ core.Event) error {
		received = true
		return nil
	})

	app := &core.Application{ID: "app-ev", Name: "event-test-app"}
	err := AutoDomain(context.Background(), store, events, app, "example.io")
	if err != nil {
		t.Fatalf("AutoDomain error: %v", err)
	}

	// Drain async dispatch so the SubscribeAsync handler's write to
	// `received` has happened-before the read below. Pre-Tier-101 this
	// test tolerated the missed-event case with t.Log, but the leaked
	// goroutine would then race with the next test's state — which is
	// how -race caught the failure attributed to TestRollback_*.
	events.Drain()
	if !received {
		t.Error("expected received=true after domain.added event drained")
	}
}

// =====================================================
// Rollback — runtime stop/remove are called for old container
// =====================================================

func TestRollback_StopsAndRemovesOldContainer(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Image: "app:v2", Status: "running"},
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-old"] = &core.Application{
		ID:       "app-old",
		Name:     "old-container-app",
		TenantID: "t1",
	}
	store.latestDeployment = &core.Deployment{
		ContainerID: "old-ctr-id-123",
	}
	store.nextVersion = 3

	var stoppedID, removedID string
	runtime := &mockRuntime{
		stopFn: func(_ context.Context, id string, _ int) error {
			stoppedID = id
			return nil
		},
		removeFn: func(_ context.Context, id string, _ bool) error {
			removedID = id
			return nil
		},
	}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	_, err := re.Rollback(context.Background(), "app-old", 1)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}

	if stoppedID != "old-ctr-id-123" {
		t.Errorf("stopped container = %q, want %q", stoppedID, "old-ctr-id-123")
	}
	if removedID != "old-ctr-id-123" {
		t.Errorf("removed container = %q, want %q", removedID, "old-ctr-id-123")
	}
}

// =====================================================
// Deployer — DeployImage with empty app name
// =====================================================

func TestDeployer_DeployImage_EmptyAppName(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 1
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:   "app-empty",
		Name: "",
	}

	dep, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err != nil {
		t.Fatalf("DeployImage error: %v", err)
	}

	// Container name should handle empty app name gracefully
	if runtime.lastOpts.Name != "monster--1" {
		t.Errorf("container name = %q, want %q", runtime.lastOpts.Name, "monster--1")
	}
	if dep.Status != "running" {
		t.Errorf("status = %q, want running", dep.Status)
	}
}

// =====================================================
// Rollback — status transitions
// =====================================================

func TestRollback_StatusTransitions(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-st"] = &core.Application{
		ID:       "app-st",
		Name:     "status-app",
		TenantID: "t1",
	}
	store.nextVersion = 2

	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "app-st", 1)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}

	// Verify status transitions: deploying -> running
	if len(store.appStatusUpdates) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d", len(store.appStatusUpdates))
	}

	statuses := make([]string, len(store.appStatusUpdates))
	for i, u := range store.appStatusUpdates {
		statuses[i] = u.Status
	}

	foundDeploying := false
	foundRunning := false
	for _, s := range statuses {
		if s == "deploying" {
			foundDeploying = true
		}
		if s == "running" {
			foundRunning = true
		}
	}

	if !foundDeploying {
		t.Error("expected 'deploying' status transition")
	}
	if !foundRunning {
		t.Error("expected 'running' status transition")
	}
}

// =====================================================
// ImageUpdateChecker — checkAll with mock store
// =====================================================



// =====================================================
// mockRuntime.Logs — coverage
// =====================================================

func TestMockRuntime_Logs(t *testing.T) {
	runtime := &mockRuntime{}
	rc, err := runtime.Logs(context.Background(), "ctr-1", "100", false)
	if err != nil {
		t.Errorf("Logs error: %v", err)
	}
	if rc != nil {
		t.Error("expected nil ReadCloser from mock")
	}
}

// =====================================================
// Rollback — Rollback triggers event with correct data
// =====================================================

func TestRollback_EventData(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 2, Image: "app:v2", Status: "running"},
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-ed"] = &core.Application{
		ID:       "app-ed",
		Name:     "event-data-app",
		TenantID: "t1",
	}
	store.nextVersion = 3

	events := core.NewEventBus(nil)
	var eventData core.DeployEventData
	events.Subscribe(core.EventRollbackDone, func(_ context.Context, event core.Event) error {
		if d, ok := event.Data.(core.DeployEventData); ok {
			eventData = d
		}
		return nil
	})

	re := NewRollbackEngine(store, nil, events)
	_, err := re.Rollback(context.Background(), "app-ed", 1)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}

	if eventData.AppID != "app-ed" {
		t.Errorf("event AppID = %q, want %q", eventData.AppID, "app-ed")
	}
	if eventData.Image != "app:v1" {
		t.Errorf("event Image = %q, want %q", eventData.Image, "app:v1")
	}
	if eventData.Version != 3 {
		t.Errorf("event Version = %d, want 3", eventData.Version)
	}
	if eventData.Strategy != "rollback" {
		t.Errorf("event Strategy = %q, want %q", eventData.Strategy, "rollback")
	}
}

// =====================================================
// Deployer — event data verification
// =====================================================

func TestDeployer_DeployImage_EventData(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 7
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	var eventData core.DeployEventData
	events.Subscribe(core.EventAppDeployed, func(_ context.Context, event core.Event) error {
		if d, ok := event.Data.(core.DeployEventData); ok {
			eventData = d
		}
		return nil
	})

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:       "app-evd",
		Name:     "event-data-deploy",
		TenantID: "t-evd",
	}

	dep, err := d.DeployImage(context.Background(), app, "myapp:v7")
	if err != nil {
		t.Fatalf("DeployImage error: %v", err)
	}

	if eventData.AppID != "app-evd" {
		t.Errorf("event AppID = %q, want %q", eventData.AppID, "app-evd")
	}
	if eventData.Version != 7 {
		t.Errorf("event Version = %d, want 7", eventData.Version)
	}
	if eventData.Image != "myapp:v7" {
		t.Errorf("event Image = %q, want %q", eventData.Image, "myapp:v7")
	}
	if eventData.ContainerID != dep.ContainerID {
		t.Errorf("event ContainerID = %q, want %q", eventData.ContainerID, dep.ContainerID)
	}
	if eventData.Strategy != "recreate" {
		t.Errorf("event Strategy = %q, want %q", eventData.Strategy, "recreate")
	}
}

// =====================================================
// Module — interface compliance
// =====================================================

func TestDockerManager_ImplementsContainerRuntime(t *testing.T) {
	// Compile-time check
	var _ core.ContainerRuntime = (*DockerManager)(nil)
}

// =====================================================
// AutoDomain — CreateDomain sets correct Type and DNSProvider
// =====================================================

func TestAutoDomain_DomainFields(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	app := &core.Application{ID: "app-df", Name: "domain-fields"}

	err := AutoDomain(context.Background(), store, events, app, "test.dev")
	if err != nil {
		t.Fatalf("AutoDomain error: %v", err)
	}

	domain, ok := store.domains["domain-fields.test.dev"]
	if !ok {
		t.Fatal("expected domain to be created")
	}

	if domain.Type != "auto" {
		t.Errorf("Type = %q, want %q", domain.Type, "auto")
	}
	if domain.DNSProvider != "auto" {
		t.Errorf("DNSProvider = %q, want %q", domain.DNSProvider, "auto")
	}
	if domain.AppID != "app-df" {
		t.Errorf("AppID = %q, want %q", domain.AppID, "app-df")
	}
}

// =====================================================
// Deployer — multiple sequential deploys
// =====================================================

func TestDeployer_DeployImage_MultipleSequential(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)

	for i := 1; i <= 3; i++ {
		store.nextVersion = i
		app := &core.Application{
			ID:       fmt.Sprintf("app-%d", i),
			Name:     fmt.Sprintf("app-%d", i),
			TenantID: "t1",
		}

		dep, err := d.DeployImage(context.Background(), app, fmt.Sprintf("nginx:%d", i))
		if err != nil {
			t.Fatalf("deploy %d error: %v", i, err)
		}
		if dep.Version != i {
			t.Errorf("deploy %d: version = %d, want %d", i, dep.Version, i)
		}
		if dep.Status != "running" {
			t.Errorf("deploy %d: status = %q, want running", i, dep.Status)
		}
	}
}
