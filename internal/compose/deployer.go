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
	runtime stackRuntime
	store   core.Store
	events  *core.EventBus
	logger  *slog.Logger
}

type stackRuntime interface {
	CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error)
	Stop(ctx context.Context, containerID string, timeoutSec int) error
	Remove(ctx context.Context, containerID string, force bool) error
}

// NewStackDeployer creates a new compose stack deployer.
// A nil logger is tolerated and replaced with slog.Default() — the
// marketplace handler historically passed nil here which NPE'd inside
// Deploy's first logger.Info call (tier 101: recovered by safeGo but
// still flagged in CI logs as a panic).
func NewStackDeployer(runtime stackRuntime, store core.Store, events *core.EventBus, logger *slog.Logger) *StackDeployer {
	if logger == nil {
		logger = slog.Default()
	}
	return &StackDeployer{
		runtime: runtime,
		store:   store,
		events:  events,
		logger:  logger,
	}
}

// DeployOpts holds options for deploying a compose stack.
type DeployOpts struct {
	AppID      string
	TenantID   string
	StackName  string
	Compose    *ComposeFile
	EnvVars    map[string]string
	HTTPRoutes []HTTPRoute
}

// HTTPRoute adds DeployMonster ingress labels to one compose service.
type HTTPRoute struct {
	ServiceName string
	FQDN        string
	Port        int
}

// Deploy deploys all services in the compose file in dependency order.
// On failure, any already-created services are rolled back (stopped and removed).
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

	// Track created containers for rollback on failure
	var created []string

	for _, svcName := range order {
		svc := opts.Compose.Services[svcName]
		if svc == nil {
			continue
		}

		containerID, err := d.deployService(ctx, opts, svcName, svc, stackNetwork)
		if err != nil {
			// Rollback: stop and remove already-created services
			d.logger.Error("deploy failed, rolling back stack",
				"stack", opts.StackName,
				"failed_service", svcName,
				"error", err,
				"created_count", len(created),
			)
			d.rollback(created)
			return fmt.Errorf("deploy service %s: %w", svcName, err)
		}
		created = append(created, containerID)
	}

	d.logger.Info("compose stack deployed", "stack", opts.StackName)
	return nil
}

// rollback stops and removes all containers in the provided list.
// Errors are logged but ignored to ensure cleanup proceeds.
func (d *StackDeployer) rollback(containerIDs []string) {
	for _, id := range containerIDs {
		// Stop with short timeout, force remove
		_ = d.runtime.Stop(context.Background(), id, 5)
		_ = d.runtime.Remove(context.Background(), id, true)
		d.logger.Info("rolled back container", "id", id)
	}
}

// deployService deploys a single service and returns its container ID.
func (d *StackDeployer) deployService(ctx context.Context, opts DeployOpts, svcName string, svc *ServiceConfig, network string) (string, error) {
	image := svc.Image
	if image == "" {
		if svc.Build != nil {
			return "", fmt.Errorf("service %s has build: directive — build first, then deploy", svcName)
		}
		return "", fmt.Errorf("service %s: no image specified", svcName)
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
	for _, route := range opts.HTTPRoutes {
		if route.ServiceName != svcName || route.FQDN == "" || route.Port <= 0 {
			continue
		}
		routeName := labelName(opts.StackName + "-" + svcName)
		labels["monster.http.routers."+routeName+".rule"] = fmt.Sprintf("Host(`%s`)", route.FQDN)
		labels["monster.http.services."+routeName+".loadbalancer.server.port"] = fmt.Sprintf("%d", route.Port)
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
		if svc.Deploy.Resources.Limits.CPUs != "" {
			cpuQuota = parseCPUs(svc.Deploy.Resources.Limits.CPUs)
		}
	}

	d.logger.Info("deploying service",
		"stack", opts.StackName,
		"service", svcName,
		"image", image,
		"cpuQuota", cpuQuota,
		"memoryMB", memoryMB,
	)

	containerID, err := d.runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         image,
		Env:           env,
		Labels:        labels,
		Network:       network,
		CPUQuota:      cpuQuota,
		MemoryMB:      memoryMB,
		RestartPolicy: restartPolicy,
	})

	return containerID, err
}

func labelName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "route"
	}
	return out
}

// parseMemory converts memory strings like "512m", "1g" to megabytes.
func parseMemory(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "g") || strings.HasSuffix(s, "gb") {
		s = strings.TrimRight(s, "gb")
		var val float64
		if _, err := fmt.Sscanf(s, "%f", &val); err != nil {
			return 0
		}
		return int64(val * 1024)
	}
	if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "mb") {
		s = strings.TrimRight(s, "mb")
		var val int64
		if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
			return 0
		}
		return val
	}
	return 0
}

// parseCPUs converts CPU strings like "0.5", "2" to cpu quotas (microseconds per core).
// A cpuQuota of 50000 represents 50ms, which equals 0.5 CPU with 100ms slice.
// Compose uses CPUs as a fraction of a core; we convert to Docker's cpu-quota format.
func parseCPUs(s string) int64 {
	s = strings.TrimSpace(s)
	var val float64
	if _, err := fmt.Sscanf(s, "%f", &val); err != nil {
		return 0
	}
	if val <= 0 {
		return 0
	}
	// Docker cpu-quota is in microseconds per period (default 100000 = 100ms).
	// Convert CPU fraction to quota: cpuQuota = CPUs * 100000
	return int64(val * 100000)
}
