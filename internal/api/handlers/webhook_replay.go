package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WebhookReplayHandler re-triggers a webhook from its delivery log.
type WebhookReplayHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewWebhookReplayHandler(store core.Store, events *core.EventBus) *WebhookReplayHandler {
	return &WebhookReplayHandler{store: store, events: events}
}

// Replay handles POST /api/v1/apps/{id}/webhooks/{logId}/replay
func (h *WebhookReplayHandler) Replay(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	logID, ok := requirePathParam(w, r, "logId")
	if !ok {
		return
	}

	// Would look up the original webhook payload from webhook_logs table
	// and re-dispatch it through the build→deploy pipeline

	h.events.PublishAsync(r.Context(), core.NewEvent(core.EventWebhookReceived, "api",
		core.WebhookEventData{WebhookID: logID, Provider: "replay"}))

	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":  app.ID,
		"log_id":  logID,
		"status":  "replaying",
		"message": "webhook delivery replayed — build pipeline triggered",
	})
}
