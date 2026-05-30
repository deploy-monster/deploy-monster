package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployTriggerHandler triggers manual builds and deployments.
type DeployTriggerHandler struct {
	store     core.Store
	runtime   core.ContainerRuntime
	nodes     core.NodeManager
	events    *core.EventBus
	freeze    core.BoltStorer
	buildGit  func(ctx context.Context, opts build.BuildOpts, logWriter io.Writer) (*build.BuildResult, error)
	buildRepo string
	buildPush bool
	buildUser string
	buildPass string
	serverCtx context.Context // canceled on graceful shutdown; goroutines should select on this
}

type deployRuntime interface {
	CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error)
	Stop(ctx context.Context, containerID string, timeoutSec int) error
	Remove(ctx context.Context, containerID string, force bool) error
	ListByLabels(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error)
}

type networkEnsurer interface {
	EnsureNetwork(ctx context.Context, name string) error
}

func NewDeployTriggerHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *DeployTriggerHandler {
	h := &DeployTriggerHandler{store: store, runtime: runtime, events: events, serverCtx: context.Background()}
	h.buildGit = func(ctx context.Context, opts build.BuildOpts, logWriter io.Writer) (*build.BuildResult, error) {
		builder := build.NewBuilder(h.runtime, h.events)
		return builder.Build(ctx, opts, logWriter)
	}
	return h
}

// SetServerContext sets the server-lifetime context used by background goroutines.
func (h *DeployTriggerHandler) SetServerContext(ctx context.Context) { h.serverCtx = ctx }

// SetNodeManager enables deploy placement on connected remote agents.
func (h *DeployTriggerHandler) SetNodeManager(nodes core.NodeManager) { h.nodes = nodes }

// SetBuildImageRegistry configures the registry/repository prefix used for built images.
func (h *DeployTriggerHandler) SetBuildImageRegistry(prefix string) {
	h.buildRepo = strings.Trim(strings.TrimSpace(prefix), "/")
}

// SetBuildImagePush enables pushing built images after docker build.
func (h *DeployTriggerHandler) SetBuildImagePush(enabled bool) { h.buildPush = enabled }

// SetBuildRegistryAuth configures Docker registry credentials for build/push.
func (h *DeployTriggerHandler) SetBuildRegistryAuth(username, password string) {
	h.buildUser = username
	h.buildPass = password
}

// SetDeployFreezeStore enables deploy-freeze enforcement for manual deploys.
func (h *DeployTriggerHandler) SetDeployFreezeStore(bolt core.BoltStorer) { h.freeze = bolt }

// buildDeployLabels creates container labels including HTTP routing labels from domains.
func (h *DeployTriggerHandler) buildDeployLabels(ctx context.Context, app *core.Application, version int) map[string]string {
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         app.ID,
		"monster.app.name":       app.Name,
		"monster.project":        app.ProjectID,
		"monster.tenant":         app.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", version),
	}

	// Fetch domains for this app and add HTTP routing labels
	domains, err := h.store.ListDomainsByApp(ctx, app.ID)
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

func (h *DeployTriggerHandler) publishAsync(ctx context.Context, event core.Event) {
	if h.events != nil {
		h.events.PublishAsync(ctx, event)
	}
}

func (h *DeployTriggerHandler) deployRuntimeForApp(app *core.Application) (deployRuntime, error) {
	if app != nil && app.ServerID != "" && app.ServerID != "local" {
		if h.nodes == nil {
			return nil, fmt.Errorf("server %s is not connected", app.ServerID)
		}
		exec, err := h.nodes.Get(app.ServerID)
		if err != nil {
			return nil, fmt.Errorf("server %s is not connected: %w", app.ServerID, err)
		}
		return exec, nil
	}
	if h.runtime == nil {
		return nil, fmt.Errorf("container runtime not available")
	}
	return h.runtime, nil
}

func isRemoteApp(app *core.Application) bool {
	return app != nil && app.ServerID != "" && app.ServerID != "local"
}

func imageRefHasRegistry(ref string) bool {
	for i, r := range ref {
		if r == '/' {
			first := ref[:i]
			return first == "localhost" || containsAny(first, ".:")
		}
	}
	return false
}

func buildImageTagForRegistry(prefix string, app *core.Application, commitSHA string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" || app == nil {
		return ""
	}
	tag := core.ShortID(commitSHA, 12)
	if tag == "" {
		tag = core.ShortID(core.GenerateID(), 12)
	}
	return prefix + "/" + imageNamePart(app.Name, app.ID) + ":" + tag
}

func imageNamePart(name, fallback string) string {
	source := strings.ToLower(strings.TrimSpace(name))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(fallback))
	}
	var b strings.Builder
	lastSep := false
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		case r == '.', r == '_', r == '-', r == ' ':
			if b.Len() > 0 && !lastSep {
				b.WriteByte('-')
				lastSep = true
			}
		}
	}
	part := strings.Trim(b.String(), "-")
	if part == "" {
		part = "app-" + core.ShortID(core.GenerateID(), 8)
	}
	return part
}

func containsAny(s, chars string) bool {
	for _, c := range s {
		for _, want := range chars {
			if c == want {
				return true
			}
		}
	}
	return false
}

func (h *DeployTriggerHandler) cleanupPreviousAppContainers(ctx context.Context, runtime deployRuntime, appID, keepContainerID string) {
	if runtime == nil {
		return
	}
	containers, err := runtime.ListByLabels(ctx, map[string]string{"monster.app.id": appID})
	if err != nil {
		slog.Warn("deploy: failed to list previous app containers", "app_id", appID, "error", err)
		return
	}
	for _, c := range containers {
		if c.ID == "" || c.ID == keepContainerID {
			continue
		}
		if err := runtime.Stop(ctx, c.ID, 30); err != nil {
			slog.Warn("deploy: failed to stop previous container", "app_id", appID, "container_id", c.ID, "error", err)
		}
		if err := runtime.Remove(ctx, c.ID, true); err != nil {
			slog.Warn("deploy: failed to remove previous container", "app_id", appID, "container_id", c.ID, "error", err)
		}
	}
}

func ensureDeployNetwork(ctx context.Context, runtime deployRuntime) error {
	nr, ok := runtime.(networkEnsurer)
	if !ok {
		return nil
	}
	return nr.EnsureNetwork(ctx, "monster-network")
}

// SubscribeWebhookDeploys wires inbound git webhooks to the same build+deploy
// path used by manual git deployments. Inbound webhook secrets are stored under
// the app ID, so WebhookID is treated as the target app ID.
func (h *DeployTriggerHandler) SubscribeWebhookDeploys() {
	if h.events == nil {
		return
	}
	h.events.SubscribeAsync(core.EventWebhookReceived, func(_ context.Context, event core.Event) error {
		data, ok := event.Data.(core.WebhookEventData)
		if !ok || data.WebhookID == "" {
			return nil
		}

		ctx := h.serverCtx
		app, err := h.store.GetApp(ctx, data.WebhookID)
		if err != nil {
			slog.Warn("webhook deploy: app lookup failed", "webhook_id", data.WebhookID, "error", err)
			return nil
		}
		if app.SourceType != "git" {
			slog.Info("webhook deploy: ignoring non-git app", "app_id", app.ID, "source_type", app.SourceType)
			return nil
		}
		if app.Branch != "" && data.Branch != "" && app.Branch != data.Branch {
			slog.Info("webhook deploy: branch ignored", "app_id", app.ID, "app_branch", app.Branch, "webhook_branch", data.Branch)
			return nil
		}
		if activeDeployFreeze(h.freeze, app.TenantID) {
			h.publishAsync(ctx, core.NewEvent(core.EventDeployFailed, "webhook_deploy", map[string]string{
				"app_id": app.ID,
				"error":  "deployments are currently frozen",
			}))
			return nil
		}

		if err := h.deployGitApp(ctx, app, "webhook", data.CommitSHA, io.Discard); err != nil {
			slog.Error("webhook deploy failed", "app_id", app.ID, "error", err)
		}
		return nil
	})
}

func (h *DeployTriggerHandler) deployGitApp(ctx context.Context, app *core.Application, triggeredBy, commitSHA string, logWriter io.Writer) error {
	if h.runtime == nil {
		err := fmt.Errorf("container runtime not available")
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, err.Error())
		return err
	}

	if err := h.store.UpdateAppStatus(ctx, app.ID, "building"); err != nil {
		slog.Error("deploy: failed to update app status", "app_id", app.ID, "error", err)
	}

	buildOpts := build.BuildOpts{
		AppID:      app.ID,
		AppName:    app.Name,
		SourceURL:  app.SourceURL,
		Branch:     app.Branch,
		CommitSHA:  commitSHA,
		Dockerfile: app.Dockerfile,
	}
	if isRemoteApp(app) && h.buildRepo != "" {
		buildOpts.ImageTag = buildImageTagForRegistry(h.buildRepo, app, commitSHA)
		buildOpts.Push = h.buildPush
		buildOpts.RegistryUsername = h.buildUser
		buildOpts.RegistryPassword = h.buildPass
	}
	result, err := h.buildGit(ctx, buildOpts, logWriter)
	if err != nil {
		h.failAppAndPublish(ctx, core.EventBuildFailed, app.ID, err.Error())
		return err
	}
	if isRemoteApp(app) && !imageRefHasRegistry(result.ImageTag) {
		err := fmt.Errorf("remote git deploy requires a registry-qualified image tag for %q; configure build push/pull before targeting server %s", result.ImageTag, app.ServerID)
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, err.Error())
		return err
	}

	if sErr := h.store.UpdateAppStatus(ctx, app.ID, "deploying"); sErr != nil {
		slog.Error("deploy: failed to update app status", "app_id", app.ID, "error", sErr)
	}
	deployRT, err := h.deployRuntimeForApp(app)
	if err != nil {
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, err.Error())
		return err
	}

	// Reserve the version AND insert the deployment row atomically up front
	// (RACE-002b). Previously the version was allocated here but the row was
	// only written after the container started, leaving a wide window in which
	// a concurrent deploy of the same app read the same MAX(version). The row
	// is created in "deploying" state and finalized (container id + status)
	// once the container is running.
	dep := &core.Deployment{
		AppID:       app.ID,
		Image:       result.ImageTag,
		CommitSHA:   result.CommitSHA,
		Status:      "deploying",
		TriggeredBy: triggeredBy,
		Strategy:    "recreate",
	}
	if err := h.store.CreateDeploymentAtomicVersion(ctx, dep); err != nil {
		slog.Error("deploy: failed to reserve deployment version", "app_id", app.ID, "error", err)
		// Row not reserved: use failAppAndPublish (not failReserved).
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, err.Error())
		return err
	}
	version := dep.Version

	labels := h.buildDeployLabels(ctx, app, version)
	containerName := fmt.Sprintf("dm-%s-%d", app.ID, version)
	if err := ensureDeployNetwork(ctx, deployRT); err != nil {
		h.failReserved(ctx, app.ID, dep, err.Error())
		return err
	}
	containerID, err := deployRT.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         result.ImageTag,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		h.failReserved(ctx, app.ID, dep, err.Error())
		return err
	}
	h.cleanupPreviousAppContainers(ctx, deployRT, app.ID, containerID)

	// Finalize the reserved deployment row now that the container is running.
	dep.ContainerID = containerID
	dep.Status = "running"
	if err := h.store.UpdateDeployment(ctx, dep); err != nil {
		slog.Error("deploy: failed to update deployment", "app_id", app.ID, "error", err)
	}
	if err := h.store.UpdateAppStatus(ctx, app.ID, "running"); err != nil {
		slog.Error("deploy: failed to update app status", "app_id", app.ID, "error", err)
	}
	h.publishAsync(ctx, core.NewEvent(core.EventAppDeployed, "deploy_trigger", core.DeployEventData{
		AppID:        app.ID,
		DeploymentID: dep.ID,
		Version:      version,
		Image:        result.ImageTag,
		ContainerID:  containerID,
		Strategy:     "recreate",
		CommitSHA:    result.CommitSHA,
	}))
	return nil
}

// failReservedDeployment marks both the app and the reserved deployment row
// as failed when a deploy aborts after the row was reserved (RACE-002b). It
// keeps the deployments table truthful so the UI and the restart-storm reclaim
// sweep don't see a row stuck in "deploying".
func (h *DeployTriggerHandler) failReservedDeployment(ctx context.Context, appID string, dep *core.Deployment) {
	if sErr := h.store.UpdateAppStatus(ctx, appID, "failed"); sErr != nil {
		slog.Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
	}
	if dep != nil && dep.ID != "" {
		now := time.Now()
		dep.Status = "failed"
		dep.FinishedAt = &now
		if uErr := h.store.UpdateDeployment(ctx, dep); uErr != nil {
			slog.Error("deploy: failed to mark deployment failed", "app_id", appID, "error", uErr)
		}
	}
}

// failReserved marks the app and the reserved deployment row as failed, then
// emits EventDeployFailed. Use when a deploy aborts after CreateDeploymentAtomicVersion
// succeeded (RACE-002b path).
//
// P1-7: Merges the failReservedDeployment+publishAsync(EventDeployFailed) pair
// that was copy-pasted at three sites in deployGitApp.
func (h *DeployTriggerHandler) failReserved(ctx context.Context, appID string, dep *core.Deployment, errMsg string) {
	h.failReservedDeployment(ctx, appID, dep)
	h.publishDeployFailed(ctx, "deploy_trigger", appID, errMsg)
}

// failApp marks the app as failed. Use when the deployment row has not yet been reserved.
// P1-7: Extracted from 7× inline copy-paste blocks in deployGitApp.
func (h *DeployTriggerHandler) failApp(ctx context.Context, appID string) {
	if sErr := h.store.UpdateAppStatus(ctx, appID, "failed"); sErr != nil {
		slog.Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
	}
}

// failAppAndPublish marks the app as failed and emits the appropriate event.
// P1-7: Consolidates the failApp+publishAsync(EventDeployFailed) pair used at 4 sites.
func (h *DeployTriggerHandler) failAppAndPublish(ctx context.Context, eventType string, appID, errMsg string) {
	h.failApp(ctx, appID)
	h.publishAsync(ctx, core.NewEvent(eventType, "deploy_trigger", map[string]string{
		"app_id": appID,
		"error":  errMsg,
	}))
}

// publishDeployFailed emits an EventDeployFailed event.
// P1-7: Centralizes the publishAsync(EventDeployFailed) call that was copy-pasted 7×.
func (h *DeployTriggerHandler) publishDeployFailed(ctx context.Context, source, appID, errMsg string) {
	h.publishAsync(ctx, core.NewEvent(core.EventDeployFailed, source, map[string]string{
		"app_id": appID,
		"error":  errMsg,
	}))
}

// TriggerDeploy handles POST /api/v1/apps/{id}/deploy
// Triggers a manual build+deploy for a git-sourced app.
func (h *DeployTriggerHandler) TriggerDeploy(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if activeDeployFreeze(h.freeze, app.TenantID) {
		writeError(w, http.StatusLocked, "deployments are currently frozen")
		return
	}

	if app.SourceType == "image" {
		// For image-type apps, just redeploy the same image
		if err := h.store.UpdateAppStatus(r.Context(), appID, "deploying"); err != nil {
			slog.Error("deploy: failed to update app status", "app_id", appID, "error", err)
		}

		dep := &core.Deployment{
			AppID:       appID,
			Image:       app.SourceURL,
			Status:      "deploying",
			TriggeredBy: "manual",
			Strategy:    "recreate",
		}
		// Reserve the version and insert the row atomically (RACE-002b) so a
		// concurrent deploy of the same app cannot claim the same version.
		if err := h.store.CreateDeploymentAtomicVersion(r.Context(), dep); err != nil {
			slog.Error("deploy: failed to create deployment", "app_id", appID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		version := dep.Version

		deployRT, err := h.deployRuntimeForApp(app)
		if err == nil {
			// Build labels with HTTP routing from domains
			labels := h.buildDeployLabels(r.Context(), app, version)

			containerName := fmt.Sprintf("dm-%s-%d", app.ID, version)
			if err := ensureDeployNetwork(r.Context(), deployRT); err != nil {
				if sErr := h.store.UpdateAppStatus(r.Context(), appID, "failed"); sErr != nil {
					ctxLogger(r.Context()).Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
				}
				internalErrorCtx(r.Context(), w, "deploy failed", err)
				return
			}
			containerID, err := deployRT.CreateAndStart(r.Context(), core.ContainerOpts{
				Name:          containerName,
				Image:         app.SourceURL,
				Labels:        labels,
				Network:       "monster-network",
				RestartPolicy: "unless-stopped",
			})
			if err != nil {
				if sErr := h.store.UpdateAppStatus(r.Context(), appID, "failed"); sErr != nil {
					ctxLogger(r.Context()).Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
				}
				internalErrorCtx(r.Context(), w, "deploy failed", err)
				return
			}
			dep.ContainerID = containerID
			if err := h.store.UpdateDeployment(r.Context(), dep); err != nil {
				slog.Error("deploy: failed to update deployment container", "app_id", appID, "error", err)
			}
			h.cleanupPreviousAppContainers(r.Context(), deployRT, app.ID, containerID)
		} else if app.ServerID != "" {
			if sErr := h.store.UpdateAppStatus(r.Context(), appID, "failed"); sErr != nil {
				ctxLogger(r.Context()).Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
			}
			internalErrorCtx(r.Context(), w, "deploy failed", err)
			return
		}

		if err := h.store.UpdateAppStatus(r.Context(), appID, "running"); err != nil {
			slog.Error("deploy: failed to update app status", "app_id", appID, "error", err)
		}
		h.publishAsync(r.Context(), core.NewEvent(core.EventAppDeployed, "deploy_trigger", core.DeployEventData{
			AppID:        appID,
			DeploymentID: dep.ID,
			Version:      version,
			Image:        app.SourceURL,
			ContainerID:  dep.ContainerID,
			Strategy:     "recreate",
		}))

		writeJSON(w, http.StatusOK, map[string]any{
			"deployment": dep,
			"status":     "deployed",
		})
		return
	}

	// Use server-scoped context — outlives the request but cancels on shutdown
	safeGo(func() {
		_ = h.deployGitApp(h.serverCtx, app, "manual", "", io.Discard)
	}, func(recovered any) {
		h.store.UpdateAppStatus(h.serverCtx, appID, "failed")
		h.publishAsync(h.serverCtx, core.NewEvent(core.EventDeployFailed, "deploy_trigger", map[string]string{
			"app_id": appID,
			"error":  fmt.Sprintf("panic: %v", recovered),
		}))
	})

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "building",
		"message": "build and deploy pipeline triggered",
	})
}
