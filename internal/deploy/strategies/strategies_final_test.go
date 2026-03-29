package strategies

import (
	"context"
	"io"
	"testing"
	"time"

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

// ═══════════════════════════════════════════════════════════════════════════════
// buildLabels — covers domain routing labels
// ═══════════════════════════════════════════════════════════════════════════════

func TestBuildLabels_WithDomains(t *testing.T) {
	runtime := &mockRuntime{}
	store := &mockStore{
		domains: []core.Domain{
			{FQDN: "app.example.com"},
			{FQDN: "api.example.com"},
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-domains",
			Name:     "myapp",
			TenantID: "tenant-1",
			Port:     3000,
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "myapp:v1",
		OldContainerID: "",
		Runtime:        runtime,
		Store:          store,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	opts := runtime.lastOpts

	// Check basic labels
	if opts.Labels["monster.enable"] != "true" {
		t.Error("missing monster.enable label")
	}

	// Check domain routing labels
	router1 := "myapp-0"
	if opts.Labels["monster.http.routers."+router1+".rule"] != "Host(`app.example.com`)" {
		t.Errorf("router rule for domain 1 = %q, want Host(`app.example.com`)",
			opts.Labels["monster.http.routers."+router1+".rule"])
	}
	if opts.Labels["monster.http.services."+router1+".loadbalancer.server.port"] != "3000" {
		t.Errorf("service port for domain 1 = %q, want 3000",
			opts.Labels["monster.http.services."+router1+".loadbalancer.server.port"])
	}

	router2 := "myapp-1"
	if opts.Labels["monster.http.routers."+router2+".rule"] != "Host(`api.example.com`)" {
		t.Errorf("router rule for domain 2 = %q, want Host(`api.example.com`)",
			opts.Labels["monster.http.routers."+router2+".rule"])
	}
}

func TestBuildLabels_DefaultPort(t *testing.T) {
	runtime := &mockRuntime{}
	store := &mockStore{
		domains: []core.Domain{
			{FQDN: "defaultport.example.com"},
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-default-port",
			Name:     "defaultport",
			TenantID: "tenant-1",
			Port:     0, // Should default to 80
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "app:v1",
		OldContainerID: "",
		Runtime:        runtime,
		Store:          store,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	opts := runtime.lastOpts
	router := "defaultport-0"
	if opts.Labels["monster.http.services."+router+".loadbalancer.server.port"] != "80" {
		t.Errorf("service port = %q, want 80 (default)",
			opts.Labels["monster.http.services."+router+".loadbalancer.server.port"])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Health Check Scenarios
// ═══════════════════════════════════════════════════════════════════════════════

// mockRuntimeNoHealthCheck returns stats without health check defined
type mockRuntimeNoHealthCheck struct {
	mockRuntime
}

func (m *mockRuntimeNoHealthCheck) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{
		Health:  "", // No health check defined
		Running: true,
	}, nil
}

func TestRolling_Execute_NoHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test with sleep in short mode")
	}

	runtime := &mockRuntimeNoHealthCheck{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-no-health",
			Name:     "nohealth",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
		Graceful: &GracefulConfig{
			StartupTimeout:      5 * time.Second,
			HealthCheckInterval: 100 * time.Millisecond,
		},
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
}

// mockRuntimeUnhealthy returns unhealthy stats initially, then healthy
type mockRuntimeUnhealthy struct {
	mockRuntime
	callCount int
}

func (m *mockRuntimeUnhealthy) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	m.callCount++
	if m.callCount < 3 {
		return &core.ContainerStats{
			Health:  "unhealthy",
			Running: true,
		}, nil
	}
	return &core.ContainerStats{
		Health:  "healthy",
		Running: true,
	}, nil
}

func TestRolling_Execute_UnhealthyThenHealthy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test with health polling in short mode")
	}

	runtime := &mockRuntimeUnhealthy{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-unhealthy",
			Name:     "unhealthy",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
		Graceful: &GracefulConfig{
			StartupTimeout:      5 * time.Second,
			HealthCheckInterval: 100 * time.Millisecond,
		},
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
}

// mockRuntimeStatsError returns error from Stats
type mockRuntimeStatsError struct {
	mockRuntime
	callCount int
}

func (m *mockRuntimeStatsError) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	m.callCount++
	if m.callCount < 3 {
		return nil, io.ErrUnexpectedEOF
	}
	return &core.ContainerStats{
		Health:  "healthy",
		Running: true,
	}, nil
}

func TestRolling_Execute_StatsErrorThenSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test with health polling in short mode")
	}

	runtime := &mockRuntimeStatsError{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-stats-err",
			Name:     "statserr",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
		Graceful: &GracefulConfig{
			StartupTimeout:      5 * time.Second,
			HealthCheckInterval: 100 * time.Millisecond,
		},
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
}

// mockRuntimeNeverHealthy never becomes healthy (for timeout test)
type mockRuntimeNeverHealthy struct {
	mockRuntime
}

func (m *mockRuntimeNeverHealthy) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{
		Health:  "starting",
		Running: true,
	}, nil
}

func (m *mockRuntimeNeverHealthy) Stop(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockRuntimeNeverHealthy) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}

func TestRolling_Execute_HealthCheckTimeout(t *testing.T) {
	runtime := &mockRuntimeNeverHealthy{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-timeout",
			Name:     "timeout",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
		Graceful: &GracefulConfig{
			StartupTimeout:      200 * time.Millisecond,
			HealthCheckInterval: 50 * time.Millisecond,
		},
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error when health check times out")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Recreate Strategy Additional Tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestRecreate_Execute_WithDomains(t *testing.T) {
	runtime := &mockRuntime{}
	store := &mockStore{
		domains: []core.Domain{
			{FQDN: "recreate.example.com"},
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-recreate-domains",
			Name:     "recreatedomains",
			TenantID: "tenant-1",
			Port:     8080,
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
		Store:          store,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Recreate.Execute returned error: %v", err)
	}

	opts := runtime.lastOpts
	router := "recreatedomains-0"
	if opts.Labels["monster.http.routers."+router+".rule"] != "Host(`recreate.example.com`)" {
		t.Errorf("router rule = %q, want Host(`recreate.example.com`)",
			opts.Labels["monster.http.routers."+router+".rule"])
	}
	if opts.Labels["monster.http.services."+router+".loadbalancer.server.port"] != "8080" {
		t.Errorf("service port = %q, want 8080",
			opts.Labels["monster.http.services."+router+".loadbalancer.server.port"])
	}
}

func TestRecreate_Execute_StopError(t *testing.T) {
	runtime := &mockRuntime{
		stopFn: func(_ context.Context, _ string, _ int) error {
			return io.ErrUnexpectedEOF
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-stop-err",
			Name:     "stoperr",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 2,
		},
		NewImage:       "nginx:v2",
		OldContainerID: "old-container-stop-err",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	// Stop error should not cause failure (non-fatal)
	if err != nil {
		t.Fatalf("Recreate.Execute should not fail on stop error, got: %v", err)
	}

	if !runtime.stopCalled {
		t.Error("Stop should be called for old container")
	}
	if !runtime.createCalled {
		t.Error("CreateAndStart should be called for new container")
	}
}
