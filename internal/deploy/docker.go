package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check: DockerManager implements core.ContainerRuntime.
var _ core.ContainerRuntime = (*DockerManager)(nil)

// DockerManager wraps the Docker SDK client for container operations.
type DockerManager struct {
	cli          *client.Client
	defaultCPU   int64 // Default CPU quota (microseconds per 100ms period)
	defaultMemMB int64 // Default memory limit in MB
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

// SetResourceDefaults configures default CPU and memory limits applied to
// containers that don't specify their own limits.
func (d *DockerManager) SetResourceDefaults(cpuQuota, memoryMB int64) {
	d.defaultCPU = cpuQuota
	d.defaultMemMB = memoryMB
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
	// Apply resource defaults for containers that don't specify limits
	opts.ApplyResourceDefaults(d.defaultCPU, d.defaultMemMB)

	// Validate volume paths before doing anything
	if err := opts.ValidateVolumePaths(); err != nil {
		return "", fmt.Errorf("invalid container opts: %w", err)
	}

	// Pull image with 5-minute timeout to prevent hanging daemon from blocking API
	pullCtx, pullCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer pullCancel()
	reader, err := d.cli.ImagePull(pullCtx, opts.Image, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("pull image %s: %w", opts.Image, err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)

	// Container config
	containerCfg := &container.Config{
		Image:  opts.Image,
		Env:    opts.Env,
		Labels: opts.Labels,
	}

	// Host config with security hardening and log rotation
	hostCfg := &container.HostConfig{
		SecurityOpt: []string{"no-new-privileges"},
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": "50m", // Rotate at 50MB
				"max-file": "5",   // Keep 5 rotated files (250MB max per container)
			},
		},
	}

	if opts.Privileged {
		hostCfg.Privileged = true
		hostCfg.SecurityOpt = nil // Privileged mode overrides security opts
	} else {
		// Drop all capabilities, add back only what's needed for typical web apps
		hostCfg.CapDrop = []string{"ALL"}
		hostCfg.CapAdd = []string{
			"CHOWN", "SETUID", "SETGID", // File ownership + process identity
			"NET_BIND_SERVICE", // Bind ports < 1024
			"DAC_OVERRIDE",     // Read/write files regardless of permission
		}
	}

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
	stopCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second+30*time.Second)
	defer cancel()
	return d.cli.ContainerStop(stopCtx, containerID, container.StopOptions{Timeout: &timeoutSec})
}

// Remove implements core.ContainerRuntime.
func (d *DockerManager) Remove(ctx context.Context, containerID string, force bool) error {
	removeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return d.cli.ContainerRemove(removeCtx, containerID, container.RemoveOptions{Force: force})
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
		return nil, fmt.Errorf("inspect container %s: %w", containerID, err)
	}
	return &resp, nil
}

// Exec runs a command inside a running container and returns the output.
func (d *DockerManager) Exec(ctx context.Context, containerID string, cmd []string) (string, error) {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := d.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := d.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, attachResp.Reader); err != nil {
		return "", fmt.Errorf("exec read: %w", err)
	}

	return buf.String(), nil
}

// Stats returns real-time resource usage statistics for a container.
func (d *DockerManager) Stats(ctx context.Context, containerID string) (*core.ContainerStats, error) {
	resp, err := d.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("container stats: %w", err)
	}
	defer resp.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}

	// Calculate CPU percentage
	cpuPercent := 0.0
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if sysDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / sysDelta) * float64(stats.CPUStats.OnlineCPUs) * 100.0
	}

	// Calculate memory percentage
	memPercent := 0.0
	if stats.MemoryStats.Limit > 0 {
		memPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	// Aggregate network stats across all interfaces
	var netRx, netTx int64
	for _, iface := range stats.Networks {
		netRx += int64(iface.RxBytes)
		netTx += int64(iface.TxBytes)
	}

	// Aggregate block I/O
	var blockRead, blockWrite int64
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch entry.Op {
		case "read", "Read":
			// Guard uint64->int64 overflow
			if entry.Value > (1<<63)-1 {
				blockRead = math.MaxInt64
			} else {
				blockRead += int64(entry.Value)
			}
		case "write", "Write":
			if entry.Value > (1<<63)-1 {
				blockWrite = math.MaxInt64
			} else {
				blockWrite += int64(entry.Value)
			}
		}
	}

	// Get container health and running status via inspect
	health := ""
	running := false
	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err == nil {
		running = inspect.State != nil && inspect.State.Running
		if inspect.State != nil && inspect.State.Health != nil {
			health = inspect.State.Health.Status
		}
	}

	return &core.ContainerStats{
		CPUPercent:    cpuPercent,
		MemoryUsage:   clampToInt64(stats.MemoryStats.Usage),
		MemoryLimit:   clampToInt64(stats.MemoryStats.Limit),
		MemoryPercent: memPercent,
		NetworkRx:     netRx,
		NetworkTx:     netTx,
		BlockRead:     blockRead,
		BlockWrite:    blockWrite,
		PIDs:          int(stats.PidsStats.Current),
		Health:        health,
		Running:       running,
	}, nil
}

// ImagePull pulls an image from a registry.
func (d *DockerManager) ImagePull(ctx context.Context, img string) error {
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	reader, err := d.cli.ImagePull(pullCtx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", img, err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// ImageList returns all images in the Docker host.
func (d *DockerManager) ImageList(ctx context.Context) ([]core.ImageInfo, error) {
	images, err := d.cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	result := make([]core.ImageInfo, len(images))
	for i, img := range images {
		result[i] = core.ImageInfo{
			ID:      img.ID,
			Tags:    img.RepoTags,
			Size:    img.Size,
			Created: img.Created,
		}
	}
	return result, nil
}

// ImageRemove removes an image from the Docker host.
func (d *DockerManager) ImageRemove(ctx context.Context, imageID string) error {
	_, err := d.cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: false, PruneChildren: true})
	if err != nil {
		return fmt.Errorf("remove image %s: %w", imageID, err)
	}
	return nil
}

// NetworkList returns all networks configured in the Docker host.
func (d *DockerManager) NetworkList(ctx context.Context) ([]core.NetworkInfo, error) {
	networks, err := d.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}

	result := make([]core.NetworkInfo, len(networks))
	for i, n := range networks {
		result[i] = core.NetworkInfo{
			ID:     n.ID,
			Name:   n.Name,
			Driver: n.Driver,
			Scope:  n.Scope,
		}
	}
	return result, nil
}

// VolumeList returns all volumes configured in the Docker host.
func (d *DockerManager) VolumeList(ctx context.Context) ([]core.VolumeInfo, error) {
	resp, err := d.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}

	result := make([]core.VolumeInfo, len(resp.Volumes))
	for i, v := range resp.Volumes {
		result[i] = core.VolumeInfo{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			CreatedAt:  v.CreatedAt,
		}
	}
	return result, nil
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

// clampToInt64 converts uint64 to int64 safely, clamping to MaxInt64 on overflow.
// This prevents uint64->int64 wraparound in metrics reporting.
func clampToInt64(v uint64) int64 {
	if v > (1<<63)-1 {
		return math.MaxInt64
	}
	return int64(v)
}
