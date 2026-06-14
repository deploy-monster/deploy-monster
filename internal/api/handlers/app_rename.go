package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RenameHandler provides a dedicated rename endpoint (simpler than full PATCH).
type RenameHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewRenameHandler(store core.Store, events *core.EventBus) *RenameHandler {
	return &RenameHandler{store: store, events: events}
}

// Rename handles POST /api/v1/apps/{id}/rename
func (h *RenameHandler) Rename(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if err := validateAppName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	oldName := app.Name
	app.Name = req.Name
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "rename failed")
		return
	}

	publishEventAsync(r.Context(), h.events, core.NewEvent(core.EventAppUpdated, "api",
		map[string]string{"app_id": appID, "old_name": oldName, "new_name": req.Name}))

	writeJSON(w, http.StatusOK, map[string]string{
		"app_id":   appID,
		"old_name": oldName,
		"new_name": req.Name,
	})
}
