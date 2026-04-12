package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployNotifyHandler configures per-app deployment notifications.
type DeployNotifyHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewDeployNotifyHandler(store core.Store, bolt core.BoltStorer) *DeployNotifyHandler {
	return &DeployNotifyHandler{store: store, bolt: bolt}
}

// DeployNotifyConfig defines what notifications to send on deployment events.
type DeployNotifyConfig struct {
	OnSuccess  []NotifyTarget `json:"on_success"`
	OnFailure  []NotifyTarget `json:"on_failure"`
	OnRollback []NotifyTarget `json:"on_rollback"`
}

// NotifyTarget defines where to send a notification.
type NotifyTarget struct {
	Channel   string `json:"channel"` // slack, discord, telegram, email, webhook
	Recipient string `json:"recipient"`
}

// Get handles GET /api/v1/apps/{id}/deploy-notifications
func (h *DeployNotifyHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg DeployNotifyConfig
	if err := h.bolt.Get("deploy_notify", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, DeployNotifyConfig{})
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/deploy-notifications
func (h *DeployNotifyHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg DeployNotifyConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.bolt.Set("deploy_notify", app.ID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save notification config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"app_id": app.ID, "config": cfg, "status": "updated"})
}
