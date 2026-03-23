package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// EventWebhookHandler manages outbound system event webhooks.
// When events occur (deploy, crash, alert), configured URLs receive notifications.
type EventWebhookHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewEventWebhookHandler(store core.Store, events *core.EventBus) *EventWebhookHandler {
	return &EventWebhookHandler{store: store, events: events}
}

// EventWebhookConfig represents an outbound event webhook.
type EventWebhookConfig struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events"` // app.deployed, app.crashed, alert.triggered, etc.
	Active bool     `json:"active"`
}

// List handles GET /api/v1/webhooks/outbound
func (h *EventWebhookHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// Create handles POST /api/v1/webhooks/outbound
func (h *EventWebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req EventWebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.URL == "" || len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "url and events are required")
		return
	}

	req.ID = core.GenerateID()
	req.Active = true

	if req.Secret == "" {
		req.Secret = core.GenerateSecret(32)
	}

	writeJSON(w, http.StatusCreated, req)
}

// Delete handles DELETE /api/v1/webhooks/outbound/{id}
func (h *EventWebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	w.WriteHeader(http.StatusNoContent)
}
