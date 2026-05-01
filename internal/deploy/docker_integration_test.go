//go:build integration

package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/moby/moby/client"
)

// requireDocker creates a real DockerManager or skips the test.
func requireDocker(t *testing.T) *DockerManager {
	t.Helper()
	dm, err := NewDockerManager("")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	t.Cleanup(func() { dm.Close() })
	return dm
}

// TestDockerIntegration_PingAndInfo verifies basic Docker connectivity.
func TestDockerIntegration_PingAndInfo(t *testing.T) {
	dm := requireDocker(t)

	if err := dm.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestDockerIntegration_PullImage pulls a small image and verifies it exists.
func TestDockerIntegration_PullImage(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	// Pull a tiny image
	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	// Verify it shows up in image list
	images, err := dm.ImageList(ctx)
	if err != nil {
		t.Fatalf("ImageList: %v", err)
	}

	found := false
	for _, img := range images {
		for _, tag := range img.Tags {
			if tag == "busybox:latest" {
				found = true
			}
		}
	}
	if !found {
		t.Error("busybox:latest not found in image list after pull")
	}
}

// TestDockerIntegration_ContainerLifecycle tests the full create→start→stop→remove cycle.
func TestDockerIntegration_ContainerLifecycle(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	// Pull image first
	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	containerName := "dm-integration-test-" + core.GenerateID()[:8]

	// Create and start
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:  containerName,
		Image: "busybox:latest",
		Env:   []string{"TEST_VAR=hello"},
		Labels: map[string]string{
			"monster.managed":  "true",
			"monster.app.id":   "integration-test",
			"monster.app.name": "test-app",
		},
	})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	if containerID == "" {
		t.Fatal("expected non-empty container ID")
	}

	// Cleanup: always remove
	defer dm.Remove(ctx, containerID, true)

	// Verify container is listed with correct labels
	containers, err := dm.ListByLabels(ctx, map[string]string{
		"monster.managed": "true",
		"monster.app.id":  "integration-test",
	})
	if err != nil {
		t.Fatalf("ListByLabels: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ID == containerID || c.ID[:12] == containerID[:12] {
			found = true
			if c.Labels["monster.app.name"] != "test-app" {
				t.Errorf("label monster.app.name = %q, want %q", c.Labels["monster.app.name"], "test-app")
			}
		}
	}
	if !found {
		t.Error("container not found via ListByLabels")
	}

	// Stop
	if err := dm.Stop(ctx, containerID, 5); err != nil {
		t.Logf("Stop (may already be exited): %v", err)
	}

	// Remove
	if err := dm.Remove(ctx, containerID, true); err != nil {
		t.Errorf("Remove: %v", err)
	}

	// Verify container is gone
	containers, err = dm.ListByLabels(ctx, map[string]string{
		"monster.app.id": "integration-test",
	})
	if err != nil {
		t.Fatalf("ListByLabels after remove: %v", err)
	}
	for _, c := range containers {
		if c.ID == containerID || c.ID[:12] == containerID[:12] {
			t.Error("container still listed after remove")
		}
	}
}

// TestDockerIntegration_ContainerRestart tests the restart operation.
func TestDockerIntegration_ContainerRestart(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	containerName := "dm-restart-test-" + core.GenerateID()[:8]
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:   containerName,
		Image:  "busybox:latest",
		Labels: map[string]string{"monster.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	defer dm.Remove(ctx, containerID, true)

	// Give container a moment to start
	time.Sleep(500 * time.Millisecond)

	// Restart
	if err := dm.Restart(ctx, containerID); err != nil {
		t.Logf("Restart (container may have already exited): %v", err)
	}
}

// TestDockerIntegration_ContainerLogs retrieves logs from a container.
func TestDockerIntegration_ContainerLogs(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	containerName := "dm-logs-test-" + core.GenerateID()[:8]
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:   containerName,
		Image:  "busybox:latest",
		Labels: map[string]string{"monster.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	defer dm.Remove(ctx, containerID, true)

	// Give container time to produce output
	time.Sleep(500 * time.Millisecond)

	// Get logs (may be empty for busybox, but shouldn't error)
	reader, err := dm.Logs(ctx, containerID, "100", false)
	if err != nil {
		t.Errorf("Logs: %v", err)
	}
	if reader != nil {
		reader.Close()
	}
}

// TestDockerIntegration_NetworkLifecycle tests network create and list.
func TestDockerIntegration_NetworkLifecycle(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	networkName := "dm-test-network-" + core.GenerateID()[:8]

	// Create network
	if err := dm.EnsureNetwork(ctx, networkName); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}

	// Cleanup
	defer func() { _, _ = dm.cli.NetworkRemove(ctx, networkName, client.NetworkRemoveOptions{}) }()

	// List networks and verify
	networks, err := dm.NetworkList(ctx)
	if err != nil {
		t.Fatalf("NetworkList: %v", err)
	}

	found := false
	for _, n := range networks {
		if n.Name == networkName {
			found = true
		}
	}
	if !found {
		t.Errorf("network %q not found in list", networkName)
	}

	// Idempotent: calling EnsureNetwork again should not error
	if err := dm.EnsureNetwork(ctx, networkName); err != nil {
		t.Errorf("EnsureNetwork (idempotent): %v", err)
	}
}

// TestDockerIntegration_ContainerWithNetwork tests creating a container in a custom network.
func TestDockerIntegration_ContainerWithNetwork(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	networkName := "dm-net-test-" + core.GenerateID()[:8]
	if err := dm.EnsureNetwork(ctx, networkName); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}
	defer func() { _, _ = dm.cli.NetworkRemove(ctx, networkName, client.NetworkRemoveOptions{}) }()

	containerName := "dm-net-container-" + core.GenerateID()[:8]
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:    containerName,
		Image:   "busybox:latest",
		Network: networkName,
		Labels:  map[string]string{"monster.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateAndStart with network: %v", err)
	}
	defer dm.Remove(ctx, containerID, true)

	if containerID == "" {
		t.Error("expected non-empty container ID")
	}
}

// TestDockerIntegration_VolumeList verifies volume listing works.
func TestDockerIntegration_VolumeList(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	// Just verify the call doesn't error — we may not have volumes
	_, err := dm.VolumeList(ctx)
	if err != nil {
		t.Errorf("VolumeList: %v", err)
	}
}

// TestDockerIntegration_ImageList verifies image listing works.
func TestDockerIntegration_ImageList(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	images, err := dm.ImageList(ctx)
	if err != nil {
		t.Fatalf("ImageList: %v", err)
	}

	// Should have at least one image (busybox from earlier tests, or system images)
	if len(images) == 0 {
		t.Log("no images found (expected at least busybox if other integration tests ran)")
	}
}
