package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ServerManageHandler handles individual server management operations.
type ServerManageHandler struct {
	services *core.Services
	store    core.Store
	events   *core.EventBus
}

func NewServerManageHandler(services *core.Services, store core.Store, events *core.EventBus) *ServerManageHandler {
	return &ServerManageHandler{services: services, store: store, events: events}
}

// Decommission handles POST /api/v1/servers/{id}/decommission
// Drains workloads, removes from swarm, optionally destroys VPS.
func (h *ServerManageHandler) Decommission(w http.ResponseWriter, r *http.Request) {
	serverID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	destroy := r.URL.Query().Get("destroy") == "true"

	h.events.Publish(r.Context(), core.NewEvent(core.EventServerRemoved, "api",
		core.ServerEventData{ServerID: serverID}))

	action := "deregistered"
	if destroy {
		action = "destroyed"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"server_id": serverID,
		"action":    action,
		"status":    "processing",
	})
}

// Reboot handles POST /api/v1/servers/{id}/reboot
func (h *ServerManageHandler) Reboot(w http.ResponseWriter, r *http.Request) {
	serverID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"server_id": serverID,
		"status":    "rebooting",
	})
}
