package handlers

import (
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
			containerName := app.Name + "-" + core.GenerateID()[:6]
			containerID, err := h.runtime.CreateAndStart(r.Context(), core.ContainerOpts{
				Name:  containerName,
				Image: app.SourceURL,
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   appID,
					"monster.app.name": app.Name,
					"monster.tenant":   app.TenantID,
				},
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

	go func() {
		builder := build.NewBuilder(h.runtime, h.events)
		result, err := builder.Build(r.Context(), build.BuildOpts{
			AppID:     app.ID,
			AppName:   app.Name,
			SourceURL: app.SourceURL,
			Branch:    app.Branch,
		}, io.Discard)

		if err != nil {
			h.store.UpdateAppStatus(r.Context(), appID, "failed")
			return
		}

		// Deploy built image
		h.store.UpdateAppStatus(r.Context(), appID, "deploying")
		version, _ := h.store.GetNextDeployVersion(r.Context(), appID)
		dep := &core.Deployment{
			AppID:       appID,
			Version:     version,
			Image:       result.ImageTag,
			CommitSHA:   result.CommitSHA,
			Status:      "running",
			TriggeredBy: "manual",
			Strategy:    "recreate",
		}
		h.store.CreateDeployment(r.Context(), dep)
		h.store.UpdateAppStatus(r.Context(), appID, "running")
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "building",
		"message": "build and deploy pipeline triggered",
	})
}
