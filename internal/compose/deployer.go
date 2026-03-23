package compose

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// StackDeployer deploys a full Docker Compose stack as DeployMonster applications.
type StackDeployer struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
	logger  *slog.Logger
}

// NewStackDeployer creates a new compose stack deployer.
func NewStackDeployer(runtime core.ContainerRuntime, store core.Store, events *core.EventBus, logger *slog.Logger) *StackDeployer {
	return &StackDeployer{
		runtime: runtime,
		store:   store,
		events:  events,
		logger:  logger,
	}
}

// DeployOpts holds options for deploying a compose stack.
type DeployOpts struct {
	AppID    string
	TenantID string
	StackName string
	Compose  *ComposeFile
	EnvVars  map[string]string
}

// Deploy deploys all services in the compose file in dependency order.
func (d *StackDeployer) Deploy(ctx context.Context, opts DeployOpts) error {
	if d.runtime == nil {
		return fmt.Errorf("container runtime not available")
	}

	stackNetwork := fmt.Sprintf("monster-%s-net", opts.StackName)

	// Create isolated network for this stack
	if nr, ok := d.runtime.(interface {
		EnsureNetwork(ctx context.Context, name string) error
	}); ok {
		if err := nr.EnsureNetwork(ctx, stackNetwork); err != nil {
			return fmt.Errorf("create stack network: %w", err)
		}
	}

	// Deploy services in dependency order
	order := opts.Compose.DependencyOrder()
	d.logger.Info("deploying compose stack",
		"stack", opts.StackName,
		"services", len(opts.Compose.Services),
		"order", order,
	)

	for _, svcName := range order {
		svc := opts.Compose.Services[svcName]
		if svc == nil {
			continue
		}

		if err := d.deployService(ctx, opts, svcName, svc, stackNetwork); err != nil {
			return fmt.Errorf("deploy service %s: %w", svcName, err)
		}
	}

	d.logger.Info("compose stack deployed", "stack", opts.StackName)
	return nil
}

func (d *StackDeployer) deployService(ctx context.Context, opts DeployOpts, svcName string, svc *ServiceConfig, network string) error {
	image := svc.Image
	if image == "" {
		if svc.Build != nil {
			return fmt.Errorf("service %s has build: directive — build first, then deploy", svcName)
		}
		return fmt.Errorf("service %s: no image specified", svcName)
	}

	// Build environment variables
	env := make([]string, 0, len(svc.ResolvedEnv)+len(opts.EnvVars))
	for k, v := range opts.EnvVars {
		env = append(env, k+"="+v)
	}
	for k, v := range svc.ResolvedEnv {
		env = append(env, k+"="+v)
	}

	// Build labels
	labels := map[string]string{
		"monster.enable":        "true",
		"monster.app.id":        opts.AppID,
		"monster.stack":         opts.StackName,
		"monster.stack.service": svcName,
		"monster.tenant":        opts.TenantID,
	}
	for k, v := range svc.Labels {
		labels[k] = v
	}

	// Container name
	containerName := fmt.Sprintf("monster-%s-%s", opts.StackName, svcName)

	// Restart policy
	restartPolicy := svc.Restart
	if restartPolicy == "" {
		restartPolicy = "unless-stopped"
	}

	// Parse resource limits
	var cpuQuota int64
	var memoryMB int64
	if svc.Deploy != nil && svc.Deploy.Resources != nil && svc.Deploy.Resources.Limits != nil {
		if svc.Deploy.Resources.Limits.Memory != "" {
			memoryMB = parseMemory(svc.Deploy.Resources.Limits.Memory)
		}
	}

	d.logger.Info("deploying service",
		"stack", opts.StackName,
		"service", svcName,
		"image", image,
	)

	_, err := d.runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         image,
		Env:           env,
		Labels:        labels,
		Network:       network,
		CPUQuota:      cpuQuota,
		MemoryMB:      memoryMB,
		RestartPolicy: restartPolicy,
	})

	return err
}

// parseMemory converts memory strings like "512m", "1g" to megabytes.
func parseMemory(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "g") || strings.HasSuffix(s, "gb") {
		s = strings.TrimRight(s, "gb")
		var val float64
		fmt.Sscanf(s, "%f", &val)
		return int64(val * 1024)
	}
	if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "mb") {
		s = strings.TrimRight(s, "mb")
		var val int64
		fmt.Sscanf(s, "%d", &val)
		return val
	}
	return 0
}
