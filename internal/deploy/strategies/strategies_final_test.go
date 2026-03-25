package strategies

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Rolling.Execute — covers strategy.go:84 remaining branches
// The existing tests cover: no old container, create fails.
// Missing: Execute with an old container (stop + remove old after new starts).
// ═══════════════════════════════════════════════════════════════════════════════

func TestRolling_Execute_WithOldContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rolling deploy test with sleep in short mode")
	}

	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "rolling-old",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 3,
		},
		NewImage:       "nginx:1.25",
		OldContainerID: "old-container-789",
		Runtime:        runtime,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should have been called for new container")
	}
	if !runtime.stopCalled {
		t.Error("Stop should be called for old container")
	}
	if !runtime.removeCalled {
		t.Error("Remove should be called for old container")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRolling_Execute_WithOldContainer_Labels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rolling deploy test with sleep in short mode")
	}

	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-55",
			Name:     "rolling-labels-old",
			TenantID: "tenant-3",
		},
		Deployment: &core.Deployment{
			Version: 10,
		},
		NewImage:       "myapp:v10",
		OldContainerID: "old-c-555",
		Runtime:        runtime,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	opts := runtime.lastOpts
	if opts.Labels["monster.enable"] != "true" {
		t.Error("missing monster.enable label")
	}
	if opts.Labels["monster.app.id"] != "app-55" {
		t.Errorf("monster.app.id = %q, want app-55", opts.Labels["monster.app.id"])
	}
	if opts.Labels["monster.tenant"] != "tenant-3" {
		t.Errorf("monster.tenant = %q, want tenant-3", opts.Labels["monster.tenant"])
	}
	if opts.Labels["monster.deploy.version"] != "10" {
		t.Errorf("monster.deploy.version = %q, want 10", opts.Labels["monster.deploy.version"])
	}
	if opts.Name != "monster-rolling-labels-old-10" {
		t.Errorf("container name = %q, want monster-rolling-labels-old-10", opts.Name)
	}
}
