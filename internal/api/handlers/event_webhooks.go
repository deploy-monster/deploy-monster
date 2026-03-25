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
	bolt   core.BoltStorer
}

func NewEventWebhookHandler(store core.Store, events *core.EventBus, bolt core.BoltStorer) *EventWebhookHandler {
	return &EventWebhookHandler{store: store, events: events, bolt: bolt}
}

// EventWebhookConfig represents an outbound event webhook.
type EventWebhookConfig struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events"` // app.deployed, app.crashed, alert.triggered, etc.
	Active bool     `json:"active"`
}

// eventWebhookList wraps the persisted list of outbound webhook configs.
type eventWebhookList struct {
	Webhooks []EventWebhookConfig `json:"webhooks"`
}

// List handles GET /api/v1/webhooks/outbound
func (h *EventWebhookHandler) List(w http.ResponseWriter, _ *http.Request) {
	var list eventWebhookList
	_ = h.bolt.Get("event_webhooks", "all", &list)

	// Mask secrets in response
	safe := make([]EventWebhookConfig, len(list.Webhooks))
	for i, wh := range list.Webhooks {
		safe[i] = wh
		if safe[i].Secret != "" {
			safe[i].Secret = "****"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safe, "total": len(safe)})
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

	var list eventWebhookList
	_ = h.bolt.Get("event_webhooks", "all", &list)

	list.Webhooks = append(list.Webhooks, req)

	if err := h.bolt.Set("event_webhooks", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save webhook config")
		return
	}

	writeJSON(w, http.StatusCreated, req)
}

// Delete handles DELETE /api/v1/webhooks/outbound/{id}
func (h *EventWebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var list eventWebhookList
	if err := h.bolt.Get("event_webhooks", "all", &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	filtered := make([]EventWebhookConfig, 0, len(list.Webhooks))
	for _, wh := range list.Webhooks {
		if wh.ID != id {
			filtered = append(filtered, wh)
		}
	}
	list.Webhooks = filtered

	if err := h.bolt.Set("event_webhooks", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook configs")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
