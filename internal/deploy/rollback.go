package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RollbackEngine manages deployment version history and rollback operations.
type RollbackEngine struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

// NewRollbackEngine creates a rollback engine.
func NewRollbackEngine(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *RollbackEngine {
	return &RollbackEngine{store: store, runtime: runtime, events: events}
}

// Rollback redeploys a specific previous version of an application.
// It creates a new deployment record pointing to the old image (for audit trail).
func (r *RollbackEngine) Rollback(ctx context.Context, appID string, targetVersion int) (*core.Deployment, error) {
	// Get the target deployment to find its image
	deployments, err := r.store.ListDeploymentsByApp(ctx, appID, 50)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var targetDeploy *core.Deployment
	for i := range deployments {
		if deployments[i].Version == targetVersion {
			targetDeploy = &deployments[i]
			break
		}
	}

	if targetDeploy == nil {
		return nil, fmt.Errorf("deployment version %d not found", targetVersion)
	}

	if targetDeploy.Image == "" {
		return nil, fmt.Errorf("version %d has no image to rollback to", targetVersion)
	}

	// Get app
	app, err := r.store.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	// Get current running container
	currentDeploy, _ := r.store.GetLatestDeployment(ctx, appID)
	oldContainerID := ""
	if currentDeploy != nil {
		oldContainerID = currentDeploy.ContainerID
	}

	// Create new deployment version pointing to old image.
	// RACE-002: atomic allocation — concurrent rollbacks would otherwise
	// collide on the same version number.
	newVersion, err := r.store.AtomicNextDeployVersion(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("allocate rollback version: %w", err)
	}
	now := time.Now()
	deployment := &core.Deployment{
		AppID:         appID,
		Version:       newVersion,
		Image:         targetDeploy.Image,
		Status:        "deploying",
		CommitSHA:     targetDeploy.CommitSHA,
		CommitMessage: fmt.Sprintf("Rollback to v%d", targetVersion),
		TriggeredBy:   "rollback",
		Strategy:      "recreate",
		StartedAt:     &now,
	}
	if err := r.store.CreateDeployment(ctx, deployment); err != nil {
		return nil, fmt.Errorf("create rollback deployment: %w", err)
	}
	_ = r.store.UpdateAppStatus(ctx, appID, "deploying")

	// Stop old container
	if oldContainerID != "" && r.runtime != nil {
		_ = r.runtime.Stop(ctx, oldContainerID, 30)
		_ = r.runtime.Remove(ctx, oldContainerID, true)
	}

	// Get domains for routing labels
	domains, _ := r.store.ListDomainsByApp(ctx, appID)
	port := app.Port
	if port <= 0 {
		port = 80
	}

	// Build labels including routing labels
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         appID,
		"monster.app.name":       app.Name,
		"monster.tenant":         app.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", newVersion),
		"monster.rollback.from":  fmt.Sprintf("%d", targetVersion),
	}

	// Add HTTP routing labels for each domain
	for i, domain := range domains {
		routerName := fmt.Sprintf("%s-%d", app.Name, i)
		labels[fmt.Sprintf("monster.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain.FQDN)
		labels[fmt.Sprintf("monster.http.services.%s.loadbalancer.server.port", routerName)] = fmt.Sprintf("%d", port)
	}

	// Start new container with old image
	if r.runtime != nil {
		containerName := fmt.Sprintf("dm-%s-%d", app.ID, newVersion)
		containerID, err := r.runtime.CreateAndStart(ctx, core.ContainerOpts{
			Name:          containerName,
			Image:         targetDeploy.Image,
			Labels:        labels,
			Network:       "monster-network",
			RestartPolicy: "unless-stopped",
		})
		if err != nil {
			// Tier 100: persist the failed status so the rollback UI
			// and the restart-storm reclaim sweep see reality.
			deployment.Status = "failed"
			failed := time.Now()
			deployment.FinishedAt = &failed
			_ = r.store.UpdateDeployment(ctx, deployment)
			_ = r.store.UpdateAppStatus(ctx, appID, "failed")
			// Attempt cleanup of partially created container
			_ = r.runtime.Stop(ctx, containerName, 10)
			_ = r.runtime.Remove(ctx, containerName, true)
			return nil, fmt.Errorf("rollback deploy: %w", err)
		}
		deployment.ContainerID = containerID
	}

	deployment.Status = "running"
	finished := time.Now()
	deployment.FinishedAt = &finished
	// Tier 100: persist the running status on the deployment row.
	if err := r.store.UpdateDeployment(ctx, deployment); err != nil {
		return nil, fmt.Errorf("persist rollback deployment status: %w", err)
	}
	_ = r.store.UpdateAppStatus(ctx, appID, "running")

	_ = r.events.Publish(ctx, core.NewEvent(core.EventRollbackDone, "deploy",
		core.DeployEventData{
			AppID:        appID,
			DeploymentID: deployment.ID,
			Version:      newVersion,
			Image:        targetDeploy.Image,
			Strategy:     "rollback",
		},
	))

	return deployment, nil
}

// ListVersions returns the last N deployment versions for an app.
func (r *RollbackEngine) ListVersions(ctx context.Context, appID string, limit int) ([]VersionInfo, error) {
	deployments, err := r.store.ListDeploymentsByApp(ctx, appID, limit)
	if err != nil {
		return nil, fmt.Errorf("list versions for app %s: %w", appID, err)
	}

	versions := make([]VersionInfo, len(deployments))
	for i, d := range deployments {
		versions[i] = VersionInfo{
			Version:   d.Version,
			Image:     d.Image,
			Status:    d.Status,
			CommitSHA: d.CommitSHA,
			CreatedAt: d.CreatedAt,
			IsCurrent: i == 0,
		}
	}
	return versions, nil
}

// VersionInfo is a simplified view of a deployment for the rollback UI.
type VersionInfo struct {
	Version   int       `json:"version"`
	Image     string    `json:"image"`
	Status    string    `json:"status"`
	CommitSHA string    `json:"commit_sha,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	IsCurrent bool      `json:"is_current"`
}
