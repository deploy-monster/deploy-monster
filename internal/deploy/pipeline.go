package deploy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/webhooks"
)

// Pipeline orchestrates the full webhook -> build -> deploy flow.
// It ties together the build and deploy subsystems, emitting events at each step.
type Pipeline struct {
	store    core.Store
	builder  *build.Builder
	deployer *Deployer
	events   *core.EventBus
	logger   *slog.Logger

	// logStore captures build output per app. Before Phase 1 the
	// pipeline piped build output to io.Discard and the only way to
	// see why a build failed was to tail the server's slog sink.
	logStore *build.LogStore
}

// NewPipeline creates a new deploy pipeline.
func NewPipeline(store core.Store, runtime core.ContainerRuntime, events *core.EventBus, logger *slog.Logger) *Pipeline {
	return &Pipeline{
		store:    store,
		builder:  build.NewBuilder(runtime, events),
		deployer: NewDeployer(runtime, store, events),
		events:   events,
		logger:   logger,
		logStore: build.NewLogStore(events, 0),
	}
}

// BuildLogs returns a snapshot of the last N lines captured from the
// most recent (or in-progress) build for appID. Nil if no build has
// produced output for this app yet.
func (p *Pipeline) BuildLogs(appID string) []string {
	if p.logStore == nil {
		return nil
	}
	return p.logStore.Lines(appID)
}

// HandleWebhook processes an inbound webhook payload through the full
// build and deploy pipeline:
//  1. Find app by webhook source URL
//  2. Clone repo (builder)
//  3. Detect project type (build.Detect)
//  4. Build Docker image (builder.Build)
//  5. Deploy new version (deployer.DeployImage)
//  6. Emit events at each step
func (p *Pipeline) HandleWebhook(ctx context.Context, payload webhooks.WebhookPayload) error {
	// 1. Find app by webhook source URL
	app, err := p.findAppBySourceURL(ctx, payload.RepoURL)
	if err != nil {
		return fmt.Errorf("find app for repo %s: %w", payload.RepoURL, err)
	}

	p.logger.Info("pipeline triggered",
		"app", app.Name,
		"repo", payload.RepoURL,
		"branch", payload.Branch,
		"commit", payload.CommitSHA,
	)

	// Verify branch matches (skip if app has no branch configured)
	if app.Branch != "" && payload.Branch != "" && app.Branch != payload.Branch {
		p.logger.Info("skipping webhook: branch mismatch",
			"app_branch", app.Branch, "webhook_branch", payload.Branch)
		return nil
	}

	// 2. Emit pipeline started event
	p.events.Publish(ctx, core.NewTenantEvent(
		core.EventDeployStarted, "pipeline", app.TenantID, "",
		core.DeployEventData{AppID: app.ID, CommitSHA: payload.CommitSHA},
	))

	// 3. Update app status to building
	p.store.UpdateAppStatus(ctx, app.ID, "building")

	// Reset prior build logs for this app so the /build-logs endpoint
	// reflects only the current run.
	p.logStore.Reset(app.ID)

	// 4. Build: clone repo, detect type, generate Dockerfile, docker build.
	// Build output is captured in the per-app ring buffer and each
	// line is also published as a core.EventBuildLog for live tailing.
	result, err := p.builder.Build(ctx, build.BuildOpts{
		AppID:     app.ID,
		AppName:   app.Name,
		SourceURL: app.SourceURL,
		Branch:    payload.Branch,
		CommitSHA: payload.CommitSHA,
	}, p.logStore.Writer(app.ID))
	if err != nil {
		p.store.UpdateAppStatus(ctx, app.ID, "failed")
		p.events.Publish(ctx, core.NewTenantEvent(
			core.EventDeployFailed, "pipeline", app.TenantID, "",
			core.DeployEventData{AppID: app.ID, CommitSHA: payload.CommitSHA, Error: err.Error()},
		))
		return fmt.Errorf("build failed for %s: %w", app.Name, err)
	}

	// 5. Deploy the built image
	deployment, err := p.deployer.DeployImage(ctx, app, result.ImageTag)
	if err != nil {
		p.store.UpdateAppStatus(ctx, app.ID, "failed")
		p.events.Publish(ctx, core.NewTenantEvent(
			core.EventDeployFailed, "pipeline", app.TenantID, "",
			core.DeployEventData{AppID: app.ID, CommitSHA: payload.CommitSHA, Error: err.Error()},
		))
		return fmt.Errorf("deploy failed for %s: %w", app.Name, err)
	}

	// 6. Emit pipeline finished event
	p.events.Publish(ctx, core.NewTenantEvent(
		core.EventDeployFinished, "pipeline", app.TenantID, "",
		core.DeployEventData{
			AppID:        app.ID,
			DeploymentID: deployment.ID,
			Version:      deployment.Version,
			Image:        result.ImageTag,
			ContainerID:  deployment.ContainerID,
			CommitSHA:    result.CommitSHA,
			Strategy:     "recreate",
		},
	))

	p.logger.Info("pipeline complete",
		"app", app.Name,
		"version", deployment.Version,
		"image", result.ImageTag,
	)

	return nil
}

// findAppBySourceURL looks up an application by its source repository URL.
// It iterates tenant apps looking for a matching SourceURL.
func (p *Pipeline) findAppBySourceURL(ctx context.Context, repoURL string) (*core.Application, error) {
	if repoURL == "" {
		return nil, fmt.Errorf("empty repository URL")
	}

	// List all tenants and their apps to find a matching source URL.
	// In a production system this would use an indexed lookup.
	tenants, _, err := p.store.ListAllTenants(ctx, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}

	for _, tenant := range tenants {
		apps, _, err := p.store.ListAppsByTenant(ctx, tenant.ID, 1000, 0)
		if err != nil {
			continue
		}
		for i := range apps {
			if apps[i].SourceURL == repoURL {
				return &apps[i], nil
			}
		}
	}

	return nil, fmt.Errorf("no app found for repository %s", repoURL)
}
