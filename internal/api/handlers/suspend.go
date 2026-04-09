package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SuspendHandler manages app suspend/resume (freeze without deleting).
type SuspendHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewSuspendHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *SuspendHandler {
	return &SuspendHandler{store: store, runtime: runtime, events: events}
}

// Suspend handles POST /api/v1/apps/{id}/suspend
// Stops the container but preserves all data and configuration.
func (h *SuspendHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if app.Status == "suspended" {
		writeError(w, http.StatusConflict, "app already suspended")
		return
	}

	// Stop container but keep it (don't remove)
	if h.runtime != nil {
		containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})
		for _, c := range containers {
			h.runtime.Stop(r.Context(), c.ID, 30)
		}
	}

	h.store.UpdateAppStatus(r.Context(), appID, "suspended")

	h.events.PublishAsync(r.Context(), core.NewEvent(core.EventAppStopped, "api",
		core.AppEventData{AppID: appID, AppName: app.Name, Status: "suspended"}))

	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "status": "suspended"})
}

// Resume handles POST /api/v1/apps/{id}/resume
// Restarts a suspended app from its existing container.
func (h *SuspendHandler) Resume(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if app.Status != "suspended" {
		writeError(w, http.StatusConflict, "app is not suspended")
		return
	}

	// Restart existing container
	if h.runtime != nil {
		containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})
		for _, c := range containers {
			h.runtime.Restart(r.Context(), c.ID)
		}
	}

	h.store.UpdateAppStatus(r.Context(), appID, "running")

	h.events.PublishAsync(r.Context(), core.NewEvent(core.EventAppStarted, "api",
		core.AppEventData{AppID: appID, AppName: app.Name, Status: "running"}))

	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "status": "running"})
}
