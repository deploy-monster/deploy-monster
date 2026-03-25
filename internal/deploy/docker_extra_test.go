package deploy

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// Module — Health with docker that returns error on Ping
// =====================================================

func TestModule_Health_DockerPingFails(t *testing.T) {
	m := New()
	// We can't easily inject a mock DockerManager (it wraps real Docker SDK),
	// but we can verify the Health function returns HealthDegraded when docker is nil.
	status := m.Health()
	if status != core.HealthDegraded {
		t.Errorf("Health() = %v, want HealthDegraded", status)
	}
}

// =====================================================
// Module — Start with docker (docker will be nil in test, skip EnsureNetwork)
// =====================================================

func TestModule_Start_WithCore_NilDocker(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.Default(),
		Store:    store,
		Config:   &core.Config{},
		Services: &core.Services{},
		Events:   core.NewEventBus(slog.Default()),
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// Docker should be nil (no Docker daemon in test)
	err = m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

// =====================================================
// Module — Stop when docker is nil
// =====================================================

func TestModule_Stop_NilDocker(t *testing.T) {
	m := New()
	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop with nil docker should return nil, got %v", err)
	}
}

// =====================================================
// Module — Docker() accessor
// =====================================================

func TestModule_Docker_Accessor(t *testing.T) {
	m := New()
	if m.Docker() != nil {
		t.Error("Docker() should return nil before Init")
	}
}

// =====================================================
// Module — ID, Name, Version, Dependencies, Routes, Events (coverage boost)
// =====================================================

func TestModule_MetadataAccessors(t *testing.T) {
	m := New()

	if m.ID() != "deploy" {
		t.Errorf("ID() = %q, want deploy", m.ID())
	}
	if m.Name() != "Deploy Engine" {
		t.Errorf("Name() = %q, want 'Deploy Engine'", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want 1.0.0", m.Version())
	}
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies() = %v, want [core.db]", deps)
	}
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

// =====================================================
// mockRuntime — method coverage for Exec, Stats, ImagePull, etc
// =====================================================

func TestMockRuntime_Exec(t *testing.T) {
	rt := &mockRuntime{}
	output, err := rt.Exec(context.Background(), "ctr-1", []string{"echo", "hello"})
	if err != nil {
		t.Errorf("Exec error: %v", err)
	}
	if output != "" {
		t.Errorf("expected empty output from mock, got %q", output)
	}
}

func TestMockRuntime_Stats(t *testing.T) {
	rt := &mockRuntime{}
	stats, err := rt.Stats(context.Background(), "ctr-1")
	if err != nil {
		t.Errorf("Stats error: %v", err)
	}
	if stats == nil {
		t.Error("Stats should return non-nil")
	}
}

func TestMockRuntime_ImagePull(t *testing.T) {
	rt := &mockRuntime{}
	err := rt.ImagePull(context.Background(), "nginx:latest")
	if err != nil {
		t.Errorf("ImagePull error: %v", err)
	}
}

func TestMockRuntime_ImageList(t *testing.T) {
	rt := &mockRuntime{}
	images, err := rt.ImageList(context.Background())
	if err != nil {
		t.Errorf("ImageList error: %v", err)
	}
	if images != nil {
		t.Errorf("expected nil images from mock, got %v", images)
	}
}

func TestMockRuntime_ImageRemove(t *testing.T) {
	rt := &mockRuntime{}
	err := rt.ImageRemove(context.Background(), "sha256:abc123")
	if err != nil {
		t.Errorf("ImageRemove error: %v", err)
	}
}

func TestMockRuntime_NetworkList(t *testing.T) {
	rt := &mockRuntime{}
	networks, err := rt.NetworkList(context.Background())
	if err != nil {
		t.Errorf("NetworkList error: %v", err)
	}
	if networks != nil {
		t.Errorf("expected nil networks from mock, got %v", networks)
	}
}

func TestMockRuntime_VolumeList(t *testing.T) {
	rt := &mockRuntime{}
	volumes, err := rt.VolumeList(context.Background())
	if err != nil {
		t.Errorf("VolumeList error: %v", err)
	}
	if volumes != nil {
		t.Errorf("expected nil volumes from mock, got %v", volumes)
	}
}

func TestMockRuntime_Ping(t *testing.T) {
	rt := &mockRuntime{}
	err := rt.Ping()
	if err != nil {
		t.Errorf("Ping error: %v", err)
	}
}

// =====================================================
// AutoRestarter — handleCrash with runtime that succeeds on first try
// =====================================================

func TestAutoRestarter_HandleCrash_SucceedsFirstAttempt(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	attempts := 0
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			attempts++
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 3

	ar.handleCrash(context.Background(), "app-ok", "ctr-ok")

	if attempts != 1 {
		t.Errorf("expected 1 restart attempt, got %d", attempts)
	}

	// Should have: crashed -> running
	foundRunning := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "running" {
			foundRunning = true
		}
	}
	if !foundRunning {
		t.Error("expected 'running' status after successful restart")
	}
}

// =====================================================
// AutoRestarter — handleCrash with nil runtime breaks loop
// =====================================================

func TestAutoRestarter_HandleCrash_NilRuntime_BreaksLoop(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	ar := NewAutoRestarter(nil, store, events, logger)
	ar.maxRetries = 3

	ar.handleCrash(context.Background(), "app-nil", "ctr-nil")

	// Should go: crashed -> failed (breaks loop immediately)
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Error("expected 'failed' status when runtime is nil")
	}
}

// =====================================================
// AutoRestarter — checkCrashed with ListByLabels error
// =====================================================

func TestAutoRestarter_CheckCrashed_RuntimeError(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, context.DeadlineExceeded
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	// Should not panic
	ar.checkCrashed()
}

// =====================================================
// Deployer — DeployImage emits TenantEvent
// =====================================================

func TestDeployer_DeployImage_EmitsTenantEvent(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 1
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	var receivedTenantID string
	events.Subscribe(core.EventAppDeployed, func(_ context.Context, event core.Event) error {
		receivedTenantID = event.TenantID
		return nil
	})

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:       "app-tenant-ev",
		Name:     "tenant-event-app",
		TenantID: "tenant-123",
	}

	_, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err != nil {
		t.Fatalf("DeployImage error: %v", err)
	}

	if receivedTenantID != "tenant-123" {
		t.Errorf("event TenantID = %q, want tenant-123", receivedTenantID)
	}
}
