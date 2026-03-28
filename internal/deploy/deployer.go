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

	// Build container labels with HTTP routing labels from domains
	labels := d.buildLabels(ctx, app, version)

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

// buildLabels creates container labels including HTTP routing labels from domains.
func (d *Deployer) buildLabels(ctx context.Context, app *core.Application, version int) map[string]string {
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         app.ID,
		"monster.app.name":       app.Name,
		"monster.project":        app.ProjectID,
		"monster.tenant":         app.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", version),
	}

	// Fetch domains for this app and add HTTP routing labels
	domains, err := d.store.ListDomainsByApp(ctx, app.ID)
	if err == nil && len(domains) > 0 {
		// Get port from app or default to 80
		port := app.Port
		if port <= 0 {
			port = 80
		}

		// Add routing labels for each domain
		for i, domain := range domains {
			routerName := fmt.Sprintf("%s-%d", app.Name, i)
			// Host rule for routing
			labels[fmt.Sprintf("monster.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain.FQDN)
			// Backend port
			labels[fmt.Sprintf("monster.http.services.%s.loadbalancer.server.port", routerName)] = fmt.Sprintf("%d", port)
		}
	}

	return labels
}
