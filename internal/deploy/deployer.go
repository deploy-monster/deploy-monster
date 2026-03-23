package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Deployer handles the deployment lifecycle.
type Deployer struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
}

// NewDeployer creates a new deployer.
func NewDeployer(runtime core.ContainerRuntime, store core.Store, events *core.EventBus) *Deployer {
	return &Deployer{
		runtime: runtime,
		store:   store,
		events:  events,
	}
}

// DeployImage deploys an application from a Docker image.
func (d *Deployer) DeployImage(ctx context.Context, app *core.Application, imageRef string) (*core.Deployment, error) {
	if d.runtime == nil {
		return nil, fmt.Errorf("container runtime not available")
	}

	// Get next version number
	version, err := d.store.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("get next version: %w", err)
	}

	// Create deployment record
	now := time.Now()
	deployment := &core.Deployment{
		AppID:       app.ID,
		Version:     version,
		Image:       imageRef,
		Status:      "deploying",
		TriggeredBy: "api",
		Strategy:    "recreate",
		StartedAt:   &now,
	}
	if err := d.store.CreateDeployment(ctx, deployment); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	// Update app status
	d.store.UpdateAppStatus(ctx, app.ID, "deploying")

	// Build container labels
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         app.ID,
		"monster.app.name":       app.Name,
		"monster.project":        app.ProjectID,
		"monster.tenant":         app.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", version),
	}

	// Create and start container via core.ContainerRuntime interface
	containerName := fmt.Sprintf("monster-%s-%d", app.Name, version)
	containerID, err := d.runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         imageRef,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		d.store.UpdateAppStatus(ctx, app.ID, "failed")
		return nil, fmt.Errorf("deploy container: %w", err)
	}

	// Update deployment with container ID
	deployment.ContainerID = containerID
	deployment.Status = "running"
	finishedAt := time.Now()
	deployment.FinishedAt = &finishedAt

	// Update app status
	d.store.UpdateAppStatus(ctx, app.ID, "running")

	// Emit event
	d.events.Publish(ctx, core.NewTenantEvent(
		core.EventAppDeployed, "deploy", app.TenantID, "",
		core.DeployEventData{
			AppID:        app.ID,
			DeploymentID: deployment.ID,
			Version:      version,
			Image:        imageRef,
			ContainerID:  containerID,
			Strategy:     "recreate",
		},
	))

	return deployment, nil
}
