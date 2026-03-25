package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// MaintenanceHandler manages app maintenance mode.
// When enabled, the ingress returns a 503 maintenance page instead of proxying.
type MaintenanceHandler struct {
	store  core.Store
	events *core.EventBus
	bolt   core.BoltStorer
}

func NewMaintenanceHandler(store core.Store, events *core.EventBus, bolt core.BoltStorer) *MaintenanceHandler {
	return &MaintenanceHandler{store: store, events: events, bolt: bolt}
}

// MaintenanceConfig holds maintenance mode settings.
type MaintenanceConfig struct {
	Enabled    bool     `json:"enabled"`
	Message    string   `json:"message,omitempty"`     // Custom message on maintenance page
	AllowedIPs []string `json:"allowed_ips,omitempty"` // IPs that bypass maintenance
}

// Get handles GET /api/v1/apps/{id}/maintenance
func (h *MaintenanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg MaintenanceConfig
	if err := h.bolt.Get("maintenance", appID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, MaintenanceConfig{Enabled: false})
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/maintenance
func (h *MaintenanceHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg MaintenanceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.bolt.Set("maintenance", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save maintenance config")
		return
	}

	action := "disabled"
	if cfg.Enabled {
		action = "enabled"
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("app.maintenance", "api",
		map[string]string{"app_id": appID, "action": action}))

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": action,
	})
}
