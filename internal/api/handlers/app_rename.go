package handlers

import (
	"encoding/json"
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
	appID := r.PathValue("id")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	oldName := app.Name
	app.Name = req.Name
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "rename failed")
		return
	}

	h.events.PublishAsync(r.Context(), core.NewEvent(core.EventAppUpdated, "api",
		map[string]string{"app_id": appID, "old_name": oldName, "new_name": req.Name}))

	writeJSON(w, http.StatusOK, map[string]string{
		"app_id":   appID,
		"old_name": oldName,
		"new_name": req.Name,
	})
}
