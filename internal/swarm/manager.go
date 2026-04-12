package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Manager handles Docker Swarm cluster operations.
type Manager struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
	logger  *slog.Logger
}

// NewManager creates a Swarm cluster manager.
func NewManager(runtime core.ContainerRuntime, store core.Store, events *core.EventBus, logger *slog.Logger) *Manager {
	return &Manager{runtime: runtime, store: store, events: events, logger: logger}
}

// SwarmInfo holds current swarm state.
type SwarmInfo struct {
	Active       bool   `json:"active"`
	NodeID       string `json:"node_id"`
	ManagerAddr  string `json:"manager_addr"`
	WorkerToken  string `json:"worker_token,omitempty"`
	ManagerToken string `json:"manager_token,omitempty"`
	Nodes        int    `json:"nodes"`
}

// Init initializes a new Docker Swarm on the current node.
func (m *Manager) Init(ctx context.Context, advertiseAddr string) (*SwarmInfo, error) {
	if m.runtime == nil {
		return nil, fmt.Errorf("container runtime not available")
	}

	m.logger.Info("initializing Docker Swarm", "advertise", advertiseAddr)

	// Run docker swarm init
	args := []string{"swarm", "init"}
	if advertiseAddr != "" {
		args = append(args, "--advertise-addr", advertiseAddr)
	}

	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("swarm init failed: %w: %s", err, string(output))
	}

	// Get swarm info after init
	return m.Info(ctx)
}

// Info returns current swarm status.
func (m *Manager) Info(ctx context.Context) (*SwarmInfo, error) {
	// Run docker info to check swarm status
	output, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .Swarm}}").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("get swarm info: %w", err)
	}

	var swarmInfo struct {
		LocalNodeState   string `json:"LocalNodeState"`
		NodeID           string `json:"NodeID"`
		Managers         int    `json:"Managers"`
		Nodes            int    `json:"Nodes"`
		ControlAvailable bool   `json:"ControlAvailable"`
	}

	if err := json.Unmarshal(output, &swarmInfo); err != nil {
		return nil, fmt.Errorf("parse swarm info: %w", err)
	}

	info := &SwarmInfo{
		Active: swarmInfo.LocalNodeState == "active",
		NodeID: swarmInfo.NodeID,
		Nodes:  swarmInfo.Nodes,
	}

	// If active and is manager, get join tokens
	if info.Active && swarmInfo.ControlAvailable {
		if workerToken, err := m.getJoinToken(ctx, "worker"); err == nil {
			info.WorkerToken = workerToken
		}
		if managerToken, err := m.getJoinToken(ctx, "manager"); err == nil {
			info.ManagerToken = managerToken
		}
	}

	return info, nil
}

// getJoinToken retrieves the join token for worker or manager.
func (m *Manager) getJoinToken(ctx context.Context, role string) (string, error) {
	output, err := exec.CommandContext(ctx, "docker", "swarm", "join-token", "-q", role).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("get %s join token: %w", role, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Join adds this node to an existing swarm.
func (m *Manager) Join(ctx context.Context, managerAddr, joinToken string) error {
	m.logger.Info("joining Docker Swarm", "manager", managerAddr)

	output, err := exec.CommandContext(ctx, "docker", "swarm", "join",
		"--token", joinToken,
		managerAddr,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("swarm join failed: %w: %s", err, string(output))
	}

	m.logger.Info("successfully joined Docker Swarm")
	return nil
}

// Leave removes this node from the swarm.
func (m *Manager) Leave(ctx context.Context, force bool) error {
	m.logger.Info("leaving Docker Swarm", "force", force)

	args := []string{"swarm", "leave"}
	if force {
		args = append(args, "--force")
	}

	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("swarm leave failed: %w: %s", err, string(output))
	}

	m.logger.Info("successfully left Docker Swarm")
	return nil
}

// DeployService deploys an application as a Swarm service with replicas.
func (m *Manager) DeployService(ctx context.Context, app *core.Application, image string, replicas int) error {
	m.logger.Info("deploying Swarm service",
		"app", app.Name,
		"image", image,
		"replicas", replicas,
	)

	// Build service name
	serviceName := fmt.Sprintf("monster-%s", app.Name)

	// Build labels for routing
	labels := []string{
		fmt.Sprintf("monster.app.id=%s", app.ID),
		fmt.Sprintf("monster.app.name=%s", app.Name),
		fmt.Sprintf("monster.tenant=%s", app.TenantID),
		"monster.enable=true",
	}

	// Get domains and add routing labels
	if m.store != nil {
		domains, err := m.store.ListDomainsByApp(ctx, app.ID)
		if err == nil && len(domains) > 0 {
			port := app.Port
			if port <= 0 {
				port = 80
			}
			for i, domain := range domains {
				routerName := fmt.Sprintf("%s-%d", app.Name, i)
				labels = append(labels,
					fmt.Sprintf("monster.http.routers.%s.rule=Host(`%s`)", routerName, domain.FQDN),
					fmt.Sprintf("monster.http.services.%s.loadbalancer.server.port=%d", routerName, port),
				)
			}
		}
	}

	// Build docker service create command
	args := []string{
		"service", "create",
		"--name", serviceName,
		"--replicas", fmt.Sprintf("%d", replicas),
		"--network", "monster-network",
		"--with-registry-auth",
	}

	// Add labels
	for _, label := range labels {
		args = append(args, "--label", label)
	}

	// Add environment variables if needed
	if app.EnvVarsEnc != "" {
		// Note: In production, decrypt and add env vars
		// args = append(args, "--env", "KEY=VALUE")
	}

	// Add image
	args = append(args, image)

	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("swarm service create failed: %w: %s", err, string(output))
	}

	m.logger.Info("Swarm service deployed", "service", serviceName)
	return nil
}

// ScaleService adjusts the replica count of a Swarm service.
func (m *Manager) ScaleService(ctx context.Context, serviceName string, replicas int) error {
	m.logger.Info("scaling Swarm service", "service", serviceName, "replicas", replicas)

	fullName := fmt.Sprintf("monster-%s", serviceName)
	output, err := exec.CommandContext(ctx, "docker", "service", "scale",
		fmt.Sprintf("%s=%d", fullName, replicas),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("swarm scale failed: %w: %s", err, string(output))
	}

	m.logger.Info("Swarm service scaled", "service", fullName, "replicas", replicas)
	return nil
}

// RemoveService removes a Swarm service.
func (m *Manager) RemoveService(ctx context.Context, serviceName string) error {
	fullName := fmt.Sprintf("monster-%s", serviceName)
	m.logger.Info("removing Swarm service", "service", fullName)

	output, err := exec.CommandContext(ctx, "docker", "service", "rm", fullName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("swarm service remove failed: %w: %s", err, string(output))
	}

	m.logger.Info("Swarm service removed", "service", fullName)
	return nil
}

// CreateOverlayNetwork creates an overlay network for cross-node communication.
func (m *Manager) CreateOverlayNetwork(ctx context.Context, name string) error {
	m.logger.Info("creating overlay network", "name", name)

	output, err := exec.CommandContext(ctx, "docker", "network", "create",
		"--driver", "overlay",
		"--attachable",
		name,
	).CombinedOutput()
	if err != nil {
		// Check if network already exists
		if strings.Contains(string(output), "already exists") {
			m.logger.Debug("overlay network already exists", "name", name)
			return nil
		}
		return fmt.Errorf("create overlay network failed: %w: %s", err, string(output))
	}

	m.logger.Info("overlay network created", "name", name)
	return nil
}

// ListServices lists all Swarm services.
func (m *Manager) ListServices(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "docker", "service", "ls", "--format", "{{.Name}}").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var services []string
	for _, line := range lines {
		if line != "" {
			services = append(services, line)
		}
	}
	return services, nil
}

// ListNodes lists all nodes in the swarm.
func (m *Manager) ListNodes(ctx context.Context) ([]NodeInfo, error) {
	output, err := exec.CommandContext(ctx, "docker", "node", "ls", "--format", "{{json .}}").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var nodes []NodeInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var raw struct {
			ID           string `json:"ID"`
			Hostname     string `json:"Hostname"`
			Status       string `json:"Status"`
			Availability string `json:"Availability"`
			Manager      string `json:"ManagerStatus"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		nodes = append(nodes, NodeInfo{
			ID:           raw.ID,
			Hostname:     raw.Hostname,
			Status:       raw.Status,
			Availability: raw.Availability,
			IsManager:    raw.Manager != "" && raw.Manager != "Worker",
		})
	}
	return nodes, nil
}

// NodeInfo holds information about a swarm node.
type NodeInfo struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	Status       string `json:"status"`
	Availability string `json:"availability"`
	IsManager    bool   `json:"is_manager"`
}
