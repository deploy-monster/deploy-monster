package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WebhookLogHandler serves webhook delivery history.
type WebhookLogHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewWebhookLogHandler(store core.Store, bolt core.BoltStorer) *WebhookLogHandler {
	return &WebhookLogHandler{store: store, bolt: bolt}
}

// WebhookDeliveryLog represents a single webhook delivery attempt.
type WebhookDeliveryLog struct {
	ID         string `json:"id"`
	WebhookID  string `json:"webhook_id"`
	Event      string `json:"event"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// webhookLogList wraps persisted webhook delivery logs for an app.
type webhookLogList struct {
	Logs []WebhookDeliveryLog `json:"logs"`
}

// List handles GET /api/v1/apps/{id}/webhooks/logs
func (h *WebhookLogHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var list webhookLogList
	if err := h.bolt.Get("webhook_logs", appID, &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  list.Logs,
		"total": len(list.Logs),
	})
}
