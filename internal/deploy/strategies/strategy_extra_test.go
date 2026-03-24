package strategies

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockRuntime implements core.ContainerRuntime for testing.
type mockRuntime struct {
	createAndStartFn func(ctx context.Context, opts core.ContainerOpts) (string, error)
	stopFn           func(ctx context.Context, containerID string, timeoutSec int) error
	removeFn         func(ctx context.Context, containerID string, force bool) error
	restartFn        func(ctx context.Context, containerID string) error
	stopCalled       bool
	removeCalled     bool
	createCalled     bool
	lastOpts         core.ContainerOpts
}

func (m *mockRuntime) Ping() error { return nil }

func (m *mockRuntime) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	m.createCalled = true
	m.lastOpts = opts
	if m.createAndStartFn != nil {
		return m.createAndStartFn(ctx, opts)
	}
	return "container-123", nil
}

func (m *mockRuntime) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	m.stopCalled = true
	if m.stopFn != nil {
		return m.stopFn(ctx, containerID, timeoutSec)
	}
	return nil
}

func (m *mockRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	m.removeCalled = true
	if m.removeFn != nil {
		return m.removeFn(ctx, containerID, force)
	}
	return nil
}

func (m *mockRuntime) Restart(ctx context.Context, containerID string) error {
	if m.restartFn != nil {
		return m.restartFn(ctx, containerID)
	}
	return nil
}

func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}

func TestRecreate_Name(t *testing.T) {
	r := &Recreate{}
	if got := r.Name(); got != "recreate" {
		t.Errorf("Recreate.Name() = %q, want %q", got, "recreate")
	}
}

func TestRolling_Name(t *testing.T) {
	r := &Rolling{}
	if got := r.Name(); got != "rolling" {
		t.Errorf("Rolling.Name() = %q, want %q", got, "rolling")
	}
}

func TestNew_InvalidStrategy(t *testing.T) {
	s := New("nonexistent-strategy")
	if s == nil {
		t.Fatal("New with invalid strategy name should return a default strategy, not nil")
	}
	if got := s.Name(); got != "recreate" {
		t.Errorf("New(%q).Name() = %q, want %q (default fallback)", "nonexistent-strategy", got, "recreate")
	}
}

func TestNew_AllStrategies(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
	}{
		{input: "recreate", wantName: "recreate"},
		{input: "rolling", wantName: "rolling"},
		{input: "", wantName: "recreate"},
		{input: "unknown", wantName: "recreate"},
		{input: "blue-green", wantName: "recreate"},
		{input: "canary", wantName: "recreate"},
		{input: "ROLLING", wantName: "recreate"},
	}

	for _, tt := range tests {
		t.Run("strategy_"+tt.input, func(t *testing.T) {
			s := New(tt.input)
			if s == nil {
				t.Fatalf("New(%q) returned nil", tt.input)
			}
			if got := s.Name(); got != tt.wantName {
				t.Errorf("New(%q).Name() = %q, want %q", tt.input, got, tt.wantName)
			}
		})
	}
}

func TestRecreate_ImplementsStrategy(t *testing.T) {
	var _ Strategy = &Recreate{}
}

func TestRolling_ImplementsStrategy(t *testing.T) {
	var _ Strategy = &Rolling{}
}

func TestRecreate_Execute_NoOldContainer(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Recreate.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should have been called")
	}
	if runtime.stopCalled {
		t.Error("Stop should not be called when no old container exists")
	}
	if runtime.removeCalled {
		t.Error("Remove should not be called when no old container exists")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRecreate_Execute_WithOldContainer(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 2,
		},
		NewImage:       "nginx:1.25",
		OldContainerID: "old-container-456",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Recreate.Execute returned error: %v", err)
	}

	if !runtime.stopCalled {
		t.Error("Stop should be called for old container")
	}
	if !runtime.removeCalled {
		t.Error("Remove should be called for old container")
	}
	if !runtime.createCalled {
		t.Error("CreateAndStart should be called for new container")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRecreate_Execute_CreateFails(t *testing.T) {
	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("image pull failed")
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "invalid-image",
		OldContainerID: "",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error when CreateAndStart fails")
	}
	if err.Error() != "start new container: image pull failed" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestRecreate_Execute_Labels(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-42",
			Name:     "label-app",
			TenantID: "tenant-7",
		},
		Deployment: &core.Deployment{
			Version: 3,
		},
		NewImage: "myimg:v3",
		Runtime:  runtime,
	}

	recreate := &Recreate{}
	if err := recreate.Execute(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opts := runtime.lastOpts
	if opts.Labels["monster.enable"] != "true" {
		t.Error("missing monster.enable label")
	}
	if opts.Labels["monster.app.id"] != "app-42" {
		t.Errorf("monster.app.id = %q, want %q", opts.Labels["monster.app.id"], "app-42")
	}
	if opts.Labels["monster.app.name"] != "label-app" {
		t.Errorf("monster.app.name = %q, want %q", opts.Labels["monster.app.name"], "label-app")
	}
	if opts.Labels["monster.tenant"] != "tenant-7" {
		t.Errorf("monster.tenant = %q, want %q", opts.Labels["monster.tenant"], "tenant-7")
	}
	if opts.Labels["monster.deploy.version"] != "3" {
		t.Errorf("monster.deploy.version = %q, want %q", opts.Labels["monster.deploy.version"], "3")
	}
	if opts.Name != "monster-label-app-3" {
		t.Errorf("container name = %q, want %q", opts.Name, "monster-label-app-3")
	}
	if opts.Image != "myimg:v3" {
		t.Errorf("image = %q, want %q", opts.Image, "myimg:v3")
	}
	if opts.Network != "monster-network" {
		t.Errorf("network = %q, want %q", opts.Network, "monster-network")
	}
	if opts.RestartPolicy != "unless-stopped" {
		t.Errorf("restart policy = %q, want %q", opts.RestartPolicy, "unless-stopped")
	}
}

func TestRecreate_Execute_StopError_NonFatal(t *testing.T) {
	runtime := &mockRuntime{
		stopFn: func(_ context.Context, _ string, _ int) error {
			return fmt.Errorf("container already stopped")
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 2,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "old-container",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	// Stop error is non-fatal, so Execute should still succeed
	if err != nil {
		t.Fatalf("Recreate.Execute should succeed even if Stop fails, got: %v", err)
	}
	if !runtime.removeCalled {
		t.Error("Remove should still be called after Stop error")
	}
}

func TestRolling_Execute_NoOldContainer(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "rolling-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
	if runtime.stopCalled {
		t.Error("Stop should not be called when no old container exists")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRolling_Execute_CreateFails(t *testing.T) {
	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("port conflict")
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "old-container",
		Runtime:        runtime,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error when CreateAndStart fails")
	}
	if err.Error() != "start new container: port conflict" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
	// Old container should not be stopped if new one failed to start
	if runtime.stopCalled {
		t.Error("old container should not be stopped when new container fails to start")
	}
}

func TestRolling_Execute_Labels(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-99",
			Name:     "rolling-labels",
			TenantID: "tenant-5",
		},
		Deployment: &core.Deployment{
			Version: 7,
		},
		NewImage: "myapp:v7",
		Runtime:  runtime,
	}

	rolling := &Rolling{}
	if err := rolling.Execute(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opts := runtime.lastOpts
	if opts.Labels["monster.enable"] != "true" {
		t.Error("missing monster.enable label")
	}
	if opts.Labels["monster.app.id"] != "app-99" {
		t.Errorf("monster.app.id = %q, want %q", opts.Labels["monster.app.id"], "app-99")
	}
	if opts.Labels["monster.deploy.version"] != "7" {
		t.Errorf("monster.deploy.version = %q, want %q", opts.Labels["monster.deploy.version"], "7")
	}
	if opts.Name != "monster-rolling-labels-7" {
		t.Errorf("container name = %q, want %q", opts.Name, "monster-rolling-labels-7")
	}
}

func TestDeployPlan_Fields(t *testing.T) {
	plan := &DeployPlan{
		App: &core.Application{
			ID:   "app-1",
			Name: "test",
		},
		Deployment: &core.Deployment{
			Version: 5,
		},
		NewImage:       "nginx:1.25",
		OldContainerID: "old-id",
	}

	if plan.App.ID != "app-1" {
		t.Errorf("App.ID = %q, want %q", plan.App.ID, "app-1")
	}
	if plan.NewImage != "nginx:1.25" {
		t.Errorf("NewImage = %q, want %q", plan.NewImage, "nginx:1.25")
	}
	if plan.OldContainerID != "old-id" {
		t.Errorf("OldContainerID = %q, want %q", plan.OldContainerID, "old-id")
	}
	if plan.Deployment.Version != 5 {
		t.Errorf("Deployment.Version = %d, want 5", plan.Deployment.Version)
	}
}
