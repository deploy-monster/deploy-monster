package webhooks

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Pipeline wires webhooks to the build→deploy flow.
// When a webhook is received and matched to an app, it triggers:
// 1. Git clone the source repository
// 2. Detect project type and build Docker image
// 3. Deploy the new image via the deploy module
type Pipeline struct {
	store   core.Store
	runtime core.ContainerRuntime
	builder *build.Builder
	events  *core.EventBus
	logger  *slog.Logger
}

// NewPipeline creates a new webhook→build→deploy pipeline.
func NewPipeline(store core.Store, runtime core.ContainerRuntime, events *core.EventBus, logger *slog.Logger) *Pipeline {
	return &Pipeline{
		store:   store,
		runtime: runtime,
		builder: build.NewBuilder(runtime, events),
		events:  events,
		logger:  logger,
	}
}

// Trigger starts the build→deploy pipeline for an app.
func (p *Pipeline) Trigger(ctx context.Context, appID string, payload *WebhookPayload) error {
	app, err := p.store.GetApp(ctx, appID)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	p.logger.Info("pipeline triggered",
		"app", app.Name,
		"branch", payload.Branch,
		"commit", payload.CommitSHA,
	)

	// Update app status
	p.store.UpdateAppStatus(ctx, appID, "building")

	// Build
	result, err := p.builder.Build(ctx, build.BuildOpts{
		AppID:     app.ID,
		AppName:   app.Name,
		SourceURL: app.SourceURL,
		Branch:    app.Branch,
		CommitSHA: payload.CommitSHA,
	}, io.Discard) // In production, stream to WebSocket/SSE
	if err != nil {
		p.store.UpdateAppStatus(ctx, appID, "failed")
		return fmt.Errorf("build failed: %w", err)
	}

	// Deploy the built image
	p.store.UpdateAppStatus(ctx, appID, "deploying")

	version, _ := p.store.GetNextDeployVersion(ctx, appID)
	deployment := &core.Deployment{
		AppID:       appID,
		Version:     version,
		Image:       result.ImageTag,
		Status:      "deploying",
		CommitSHA:   result.CommitSHA,
		TriggeredBy: "webhook",
		Strategy:    "recreate",
	}
	p.store.CreateDeployment(ctx, deployment)

	containerName := fmt.Sprintf("monster-%s-%d", app.Name, version)
	containerID, err := p.runtime.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         result.ImageTag,
		Labels: map[string]string{
			"monster.enable":         "true",
			"monster.app.id":         appID,
			"monster.app.name":       app.Name,
			"monster.tenant":         app.TenantID,
			"monster.deploy.version": fmt.Sprintf("%d", version),
		},
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		p.store.UpdateAppStatus(ctx, appID, "failed")
		return fmt.Errorf("deploy failed: %w", err)
	}

	p.store.UpdateAppStatus(ctx, appID, "running")

	p.events.Publish(ctx, core.NewEvent(core.EventAppDeployed, "pipeline",
		core.DeployEventData{
			AppID:       appID,
			DeploymentID: deployment.ID,
			Version:     version,
			Image:       result.ImageTag,
			ContainerID: containerID,
			CommitSHA:   result.CommitSHA,
			Strategy:    "recreate",
		},
	))

	p.logger.Info("pipeline complete",
		"app", app.Name,
		"version", version,
		"image", result.ImageTag,
	)

	return nil
}
