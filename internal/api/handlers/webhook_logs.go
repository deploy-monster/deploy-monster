package handlers

import (
	"net/http"
	"sort"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WebhookLogHandler serves outbound webhook delivery history.
type WebhookLogHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewWebhookLogHandler(store core.Store, bolt core.BoltStorer) *WebhookLogHandler {
	return &WebhookLogHandler{store: store, bolt: bolt}
}

// WebhookDeliveryLog represents a single webhook delivery attempt.
// The shape mirrors webhooks.DeliveryLog so the persisted JSON the
// DeliveryTracker writes round-trips back to the API surface unchanged.
type WebhookDeliveryLog struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Status    string `json:"status"` // "sent" | "failed"
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
	TenantID  string `json:"tenant_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

const deliveryLogBucket = "webhook_delivery_log"

// List handles GET /api/v1/apps/{id}/webhooks/logs.
// Returns recent outbound webhook deliveries written by the
// webhooks.DeliveryTracker. The tracker doesn't yet record app
// affinity, so we return the full delivery log scoped to the
// authenticated tenant's admin view; per-app correlation will follow
// once outbound webhook configs carry an app_id label.
func (h *WebhookLogHandler) List(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	if h.bolt == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	keys, err := h.bolt.List(deliveryLogBucket)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	logs := make([]WebhookDeliveryLog, 0, len(keys))
	for _, k := range keys {
		var entry WebhookDeliveryLog
		if h.bolt.Get(deliveryLogBucket, k, &entry) == nil {
			if entry.TenantID == app.TenantID {
				logs = append(logs, entry)
			}
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].Timestamp > logs[j].Timestamp })

	const maxLogs = 200
	if len(logs) > maxLogs {
		logs = logs[:maxLogs]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  logs,
		"total": len(logs),
	})
}
