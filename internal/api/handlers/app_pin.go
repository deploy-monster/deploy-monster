package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PinHandler manages app pinning (pin to dashboard for quick access).
type PinHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewPinHandler(store core.Store, bolt core.BoltStorer) *PinHandler {
	return &PinHandler{store: store, bolt: bolt}
}

// pinnedApps is the persisted set of pinned app IDs for a user.
type pinnedApps struct {
	AppIDs []string `json:"app_ids"`
}

// Pin handles POST /api/v1/apps/{id}/pin
func (h *PinHandler) Pin(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	claims := auth.ClaimsFromContext(r.Context())

	var pins pinnedApps
	_ = h.bolt.Get("app_pins", claims.UserID, &pins)

	// Check if already pinned
	for _, id := range pins.AppIDs {
		if id == appID {
			writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "pinned": "true"})
			return
		}
	}

	pins.AppIDs = append(pins.AppIDs, appID)

	if err := h.bolt.Set("app_pins", claims.UserID, pins, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to pin app")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "pinned": "true"})
}

// Unpin handles DELETE /api/v1/apps/{id}/pin
func (h *PinHandler) Unpin(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	claims := auth.ClaimsFromContext(r.Context())

	var pins pinnedApps
	if err := h.bolt.Get("app_pins", claims.UserID, &pins); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "pinned": "false"})
		return
	}

	filtered := make([]string, 0, len(pins.AppIDs))
	for _, id := range pins.AppIDs {
		if id != appID {
			filtered = append(filtered, id)
		}
	}
	pins.AppIDs = filtered

	if err := h.bolt.Set("app_pins", claims.UserID, pins, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unpin app")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "pinned": "false"})
}
