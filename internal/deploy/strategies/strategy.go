package strategies

import (
	"context"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Strategy defines the deployment strategy interface.
type Strategy interface {
	Name() string
	Execute(ctx context.Context, plan *DeployPlan) error
}

// DeployPlan holds all information needed to execute a deployment.
type DeployPlan struct {
	App            *core.Application
	Deployment     *core.Deployment
	NewImage       string
	OldContainerID string
	Runtime        core.ContainerRuntime
	Store          core.Store
	Events         *core.EventBus
}

// New creates a strategy by name.
func New(name string) Strategy {
	switch name {
	case "rolling":
		return &Rolling{}
	default:
		return &Recreate{}
	}
}

// Recreate stops the old container, then starts the new one.
// Simple but causes brief downtime.
type Recreate struct{}

func (r *Recreate) Name() string { return "recreate" }

func (r *Recreate) Execute(ctx context.Context, plan *DeployPlan) error {
	// 1. Stop old container if exists
	if plan.OldContainerID != "" {
		if err := plan.Runtime.Stop(ctx, plan.OldContainerID, 30); err != nil {
			// Non-fatal — old container might already be stopped
		}
		plan.Runtime.Remove(ctx, plan.OldContainerID, true)
	}

	// 2. Start new container
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         plan.App.ID,
		"monster.app.name":       plan.App.Name,
		"monster.tenant":         plan.App.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", plan.Deployment.Version),
	}

	containerName := fmt.Sprintf("monster-%s-%d", plan.App.Name, plan.Deployment.Version)
	containerID, err := plan.Runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         plan.NewImage,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("start new container: %w", err)
	}

	plan.Deployment.ContainerID = containerID
	return nil
}

// Rolling starts the new container first, waits for health, then stops the old one.
// Zero-downtime for multi-instance services.
type Rolling struct{}

func (r *Rolling) Name() string { return "rolling" }

func (r *Rolling) Execute(ctx context.Context, plan *DeployPlan) error {
	// 1. Start new container (alongside old)
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         plan.App.ID,
		"monster.app.name":       plan.App.Name,
		"monster.tenant":         plan.App.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", plan.Deployment.Version),
	}

	containerName := fmt.Sprintf("monster-%s-%d", plan.App.Name, plan.Deployment.Version)
	containerID, err := plan.Runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         plan.NewImage,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("start new container: %w", err)
	}

	plan.Deployment.ContainerID = containerID

	// 2. Wait for new container to be healthy (simple delay for now)
	time.Sleep(5 * time.Second)

	// 3. Stop old container
	if plan.OldContainerID != "" {
		plan.Runtime.Stop(ctx, plan.OldContainerID, 30)
		plan.Runtime.Remove(ctx, plan.OldContainerID, true)
	}

	return nil
}
