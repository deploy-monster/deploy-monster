package deploy

import (
	"context"
	"fmt"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestNewDeployer(t *testing.T) {
	t.Run("all nil dependencies", func(t *testing.T) {
		d := NewDeployer(nil, nil, nil)
		if d == nil {
			t.Fatal("NewDeployer returned nil")
		}
	})

	t.Run("with event bus", func(t *testing.T) {
		events := core.NewEventBus(nil)
		d := NewDeployer(nil, nil, events)
		if d == nil {
			t.Fatal("NewDeployer returned nil")
		}
		if d.events != events {
			t.Error("events field not set correctly")
		}
	})

	t.Run("fields are correctly assigned", func(t *testing.T) {
		events := core.NewEventBus(nil)
		store := newMockStore()
		runtime := &mockRuntime{}
		d := NewDeployer(runtime, store, events)
		if d.runtime != runtime {
			t.Error("runtime field mismatch")
		}
		if d.store != store {
			t.Error("store field mismatch")
		}
		if d.events != events {
			t.Error("events field mismatch")
		}
	})
}

func TestDeployer_DeployImage_NilRuntime(t *testing.T) {
	events := core.NewEventBus(nil)
	d := NewDeployer(nil, nil, events)

	app := &core.Application{
		ID:   "app-1",
		Name: "test-app",
	}

	_, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err == nil {
		t.Fatal("expected error when runtime is nil")
	}
	if err.Error() != "container runtime not available" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestDeployer_DeployImage_Success(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 1
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:        "app-1",
		Name:      "deploy-test",
		ProjectID: "proj-1",
		TenantID:  "tenant-1",
	}

	dep, err := d.DeployImage(context.Background(), app, "nginx:1.25")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}
	if dep == nil {
		t.Fatal("expected non-nil deployment")
	}
	if dep.Image != "nginx:1.25" {
		t.Errorf("Image = %q, want %q", dep.Image, "nginx:1.25")
	}
	if dep.Version != 1 {
		t.Errorf("Version = %d, want 1", dep.Version)
	}
	if dep.Status != "running" {
		t.Errorf("Status = %q, want %q", dep.Status, "running")
	}
	if dep.ContainerID != "container-new-123" {
		t.Errorf("ContainerID = %q, want %q", dep.ContainerID, "container-new-123")
	}
	if dep.TriggeredBy != "api" {
		t.Errorf("TriggeredBy = %q, want %q", dep.TriggeredBy, "api")
	}
	if dep.Strategy != "recreate" {
		t.Errorf("Strategy = %q, want %q", dep.Strategy, "recreate")
	}
	if dep.StartedAt == nil {
		t.Error("StartedAt should not be nil")
	}
	if dep.FinishedAt == nil {
		t.Error("FinishedAt should not be nil")
	}

	// Verify runtime was called with correct options
	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
	opts := runtime.lastOpts
	if opts.Image != "nginx:1.25" {
		t.Errorf("runtime image = %q, want %q", opts.Image, "nginx:1.25")
	}
	if opts.Labels["monster.app.id"] != "app-1" {
		t.Errorf("label monster.app.id = %q, want %q", opts.Labels["monster.app.id"], "app-1")
	}
	if opts.Labels["monster.project"] != "proj-1" {
		t.Errorf("label monster.project = %q, want %q", opts.Labels["monster.project"], "proj-1")
	}
	if opts.Labels["monster.tenant"] != "tenant-1" {
		t.Errorf("label monster.tenant = %q, want %q", opts.Labels["monster.tenant"], "tenant-1")
	}
	if opts.Name != "monster-deploy-test-1" {
		t.Errorf("container name = %q, want %q", opts.Name, "monster-deploy-test-1")
	}

	// Verify app status was updated
	if len(store.appStatusUpdates) < 2 {
		t.Fatalf("expected at least 2 app status updates, got %d", len(store.appStatusUpdates))
	}
	if store.appStatusUpdates[0].Status != "deploying" {
		t.Errorf("first status update = %q, want %q", store.appStatusUpdates[0].Status, "deploying")
	}
	if store.appStatusUpdates[1].Status != "running" {
		t.Errorf("second status update = %q, want %q", store.appStatusUpdates[1].Status, "running")
	}
}

func TestDeployer_DeployImage_GetNextVersionError(t *testing.T) {
	store := newMockStore()
	store.nextVersionErr = fmt.Errorf("version seq error")
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{ID: "app-1", Name: "test"}

	_, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err == nil {
		t.Fatal("expected error when GetNextDeployVersion fails")
	}
}

func TestDeployer_DeployImage_CreateDeploymentError(t *testing.T) {
	store := newMockStore()
	store.createDeployErr = fmt.Errorf("insert failed")
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{ID: "app-1", Name: "test"}

	_, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err == nil {
		t.Fatal("expected error when CreateDeployment fails")
	}
}

func TestDeployer_DeployImage_ContainerCreateFails(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("docker daemon unreachable")
		},
	}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{ID: "app-1", Name: "test", TenantID: "t1"}

	_, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err == nil {
		t.Fatal("expected error when container creation fails")
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
		t.Error("app status should be set to 'failed' when container creation fails")
	}
}
