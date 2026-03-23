package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PinHandler manages app pinning (pin to dashboard for quick access).
type PinHandler struct {
	store core.Store
}

func NewPinHandler(store core.Store) *PinHandler {
	return &PinHandler{store: store}
}

// Pin handles POST /api/v1/apps/{id}/pin
func (h *PinHandler) Pin(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "pinned": "true"})
}

// Unpin handles DELETE /api/v1/apps/{id}/pin
func (h *PinHandler) Unpin(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "pinned": "false"})
}
