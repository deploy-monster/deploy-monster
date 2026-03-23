package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WebhookTestDeliveryHandler sends a test webhook payload.
type WebhookTestDeliveryHandler struct {
	events *core.EventBus
}

func NewWebhookTestDeliveryHandler(events *core.EventBus) *WebhookTestDeliveryHandler {
	return &WebhookTestDeliveryHandler{events: events}
}

// TestDeliver handles POST /api/v1/apps/{id}/webhooks/test
// Sends a fake push event to the app's webhook endpoint.
func (h *WebhookTestDeliveryHandler) TestDeliver(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	testPayload := map[string]any{
		"event":     "push",
		"ref":       "refs/heads/main",
		"test":      true,
		"timestamp": time.Now().Format(time.RFC3339),
		"sender":    "deploymonster-test",
		"repository": map[string]string{
			"full_name": "test/repo",
		},
		"head_commit": map[string]string{
			"id":      "test-" + core.GenerateID()[:8],
			"message": "Test webhook delivery from DeployMonster",
		},
	}

	payload, _ := json.Marshal(testPayload)

	// Would actually POST to the configured webhook URL
	_ = bytes.NewReader(payload)

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":  appID,
		"status":  "delivered",
		"payload": testPayload,
	})
}
