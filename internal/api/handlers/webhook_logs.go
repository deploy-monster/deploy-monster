package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WebhookLogHandler serves webhook delivery history.
type WebhookLogHandler struct {
	store core.Store
}

func NewWebhookLogHandler(store core.Store) *WebhookLogHandler {
	return &WebhookLogHandler{store: store}
}

// List handles GET /api/v1/apps/{id}/webhooks/logs
func (h *WebhookLogHandler) List(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")

	// In production, would query webhook_logs table filtered by app
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  []any{},
		"total": 0,
	})
}
