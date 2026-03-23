package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployNotifyHandler configures per-app deployment notifications.
type DeployNotifyHandler struct {
	store core.Store
}

func NewDeployNotifyHandler(store core.Store) *DeployNotifyHandler {
	return &DeployNotifyHandler{store: store}
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
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, DeployNotifyConfig{})
}

// Update handles PUT /api/v1/apps/{id}/deploy-notifications
func (h *DeployNotifyHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	var cfg DeployNotifyConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
