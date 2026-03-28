package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployTriggerHandler triggers manual builds and deployments.
type DeployTriggerHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewDeployTriggerHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *DeployTriggerHandler {
	return &DeployTriggerHandler{store: store, runtime: runtime, events: events}
}

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

// TriggerDeploy handles POST /api/v1/apps/{id}/deploy
// Triggers a manual build+deploy for a git-sourced app.
func (h *DeployTriggerHandler) TriggerDeploy(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	if app.SourceType == "image" {
		// For image-type apps, just redeploy the same image
		h.store.UpdateAppStatus(r.Context(), appID, "deploying")

		version, _ := h.store.GetNextDeployVersion(r.Context(), appID)
		dep := &core.Deployment{
			AppID:       appID,
			Version:     version,
			Image:       app.SourceURL,
			Status:      "deploying",
			TriggeredBy: "manual",
			Strategy:    "recreate",
		}
		h.store.CreateDeployment(r.Context(), dep)

		if h.runtime != nil {
			// Build labels with HTTP routing from domains
			labels := h.buildDeployLabels(r.Context(), app, version)

			containerName := fmt.Sprintf("monster-%s-%d", app.Name, version)
			containerID, err := h.runtime.CreateAndStart(r.Context(), core.ContainerOpts{
				Name:          containerName,
				Image:         app.SourceURL,
				Labels:        labels,
				Network:       "monster-network",
				RestartPolicy: "unless-stopped",
			})
			if err != nil {
				h.store.UpdateAppStatus(r.Context(), appID, "failed")
				writeError(w, http.StatusInternalServerError, "deploy failed: "+err.Error())
				return
			}
			dep.ContainerID = containerID
		}

		h.store.UpdateAppStatus(r.Context(), appID, "running")

		writeJSON(w, http.StatusOK, map[string]any{
			"deployment": dep,
			"status":     "deployed",
		})
		return
	}

	// For git-sourced apps, trigger full build pipeline
	h.store.UpdateAppStatus(r.Context(), appID, "building")

	// Use background context to avoid cancellation when request completes
	go func() {
		ctx := context.Background()
		builder := build.NewBuilder(h.runtime, h.events)
		result, err := builder.Build(ctx, build.BuildOpts{
			AppID:     app.ID,
			AppName:   app.Name,
			SourceURL: app.SourceURL,
			Branch:    app.Branch,
		}, io.Discard)

		if err != nil {
			h.store.UpdateAppStatus(ctx, appID, "failed")
			return
		}

		// Deploy built image
		h.store.UpdateAppStatus(ctx, appID, "deploying")
		version, _ := h.store.GetNextDeployVersion(ctx, appID)
		dep := &core.Deployment{
			AppID:       appID,
			Version:     version,
			Image:       result.ImageTag,
			CommitSHA:   result.CommitSHA,
			Status:      "running",
			TriggeredBy: "manual",
			Strategy:    "recreate",
		}
		h.store.CreateDeployment(ctx, dep)
		h.store.UpdateAppStatus(ctx, appID, "running")
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "building",
		"message": "build and deploy pipeline triggered",
	})
}
