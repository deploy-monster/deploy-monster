package deploy

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check: DockerManager implements core.ContainerRuntime.
var _ core.ContainerRuntime = (*DockerManager)(nil)

// DockerManager wraps the Docker SDK client for container operations.
type DockerManager struct {
	cli *client.Client
}

// NewDockerManager creates a new Docker manager with API version negotiation.
func NewDockerManager(host string) (*DockerManager, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}
	if host != "" && host != "unix:///var/run/docker.sock" {
		opts = append(opts, client.WithHost(host))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	// Verify connection
	if _, err := cli.Ping(context.Background()); err != nil {
		cli.Close()
		return nil, fmt.Errorf("docker ping: %w", err)
	}

	return &DockerManager{cli: cli}, nil
}

// Ping verifies the Docker connection.
func (d *DockerManager) Ping() error {
	_, err := d.cli.Ping(context.Background())
	return err
}

// Close closes the Docker client.
func (d *DockerManager) Close() error {
	return d.cli.Close()
}

// CreateAndStart implements core.ContainerRuntime.
// It pulls the image, creates the container, and starts it.
func (d *DockerManager) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	// Pull image
	reader, err := d.cli.ImagePull(ctx, opts.Image, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("pull image %s: %w", opts.Image, err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	// Container config
	containerCfg := &container.Config{
		Image:  opts.Image,
		Env:    opts.Env,
		Labels: opts.Labels,
	}

	// Host config
	hostCfg := &container.HostConfig{}
	if opts.CPUQuota > 0 {
		hostCfg.Resources.CPUQuota = opts.CPUQuota
	}
	if opts.MemoryMB > 0 {
		hostCfg.Resources.Memory = opts.MemoryMB * 1024 * 1024
	}
	if opts.RestartPolicy != "" {
		hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(opts.RestartPolicy)}
	}

	// Network config
	var networkCfg *network.NetworkingConfig
	if opts.Network != "" {
		networkCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				opts.Network: {},
			},
		}
	}

	// Create container
	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, opts.Name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	// Start container
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	return resp.ID, nil
}

// Stop implements core.ContainerRuntime.
func (d *DockerManager) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	return d.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSec})
}

// Remove implements core.ContainerRuntime.
func (d *DockerManager) Remove(ctx context.Context, containerID string, force bool) error {
	return d.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: force})
}

// Restart implements core.ContainerRuntime.
func (d *DockerManager) Restart(ctx context.Context, containerID string) error {
	timeout := 10
	return d.cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// Logs implements core.ContainerRuntime.
func (d *DockerManager) Logs(ctx context.Context, containerID string, tail string, follow bool) (io.ReadCloser, error) {
	return d.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
	})
}

// ListByLabels implements core.ContainerRuntime.
func (d *DockerManager) ListByLabels(ctx context.Context, labelFilters map[string]string) ([]core.ContainerInfo, error) {
	f := filters.NewArgs()
	for k, v := range labelFilters {
		f.Add("label", k+"="+v)
	}

	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, err
	}

	result := make([]core.ContainerInfo, len(containers))
	for i, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		result[i] = core.ContainerInfo{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Labels:  c.Labels,
			Created: c.Created,
		}
	}
	return result, nil
}

// InspectContainer returns detailed container info (Docker-specific, not in interface).
func (d *DockerManager) InspectContainer(ctx context.Context, containerID string) (*container.InspectResponse, error) {
	resp, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// EnsureNetwork creates a bridge network if it doesn't exist.
func (d *DockerManager) EnsureNetwork(ctx context.Context, name string) error {
	networks, err := d.cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == name {
			return nil
		}
	}

	_, err = d.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"monster.managed": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("create network %s: %w", name, err)
	}
	return nil
}
