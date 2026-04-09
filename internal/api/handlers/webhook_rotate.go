package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WebhookRotateHandler rotates webhook signing secrets.
type WebhookRotateHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewWebhookRotateHandler(store core.Store, events *core.EventBus) *WebhookRotateHandler {
	return &WebhookRotateHandler{store: store, events: events}
}

// Rotate handles POST /api/v1/apps/{id}/webhooks/rotate
// Generates a new webhook secret and returns it (shown once).
func (h *WebhookRotateHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	newSecret := core.GenerateSecret(32)

	h.events.PublishAsync(r.Context(), core.NewEvent("webhook.rotated", "api",
		map[string]string{"app_id": app.ID}))

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":     app.ID,
		"new_secret": newSecret,
		"message":    "Webhook secret rotated. Update your Git provider's webhook configuration.",
	})
}
