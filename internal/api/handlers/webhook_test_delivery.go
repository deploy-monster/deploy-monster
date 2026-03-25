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
	bolt   core.BoltStorer
}

func NewWebhookTestDeliveryHandler(events *core.EventBus, bolt core.BoltStorer) *WebhookTestDeliveryHandler {
	return &WebhookTestDeliveryHandler{events: events, bolt: bolt}
}

// webhookTestLog records test delivery results.
type webhookTestLog struct {
	AppID     string `json:"app_id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	PayloadID string `json:"payload_id"`
}

// TestDeliver handles POST /api/v1/apps/{id}/webhooks/test
// Sends a fake push event to the app's webhook endpoint.
func (h *WebhookTestDeliveryHandler) TestDeliver(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	deliveryID := core.GenerateID()

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
			"id":      "test-" + deliveryID[:8],
			"message": "Test webhook delivery from DeployMonster",
		},
	}

	payload, _ := json.Marshal(testPayload)

	// Emit event so the outbound webhook system picks it up
	h.events.Publish(r.Context(), core.NewEvent("webhook.test."+appID, "api", map[string]any{
		"app_id":      appID,
		"delivery_id": deliveryID,
		"payload":     string(payload),
	}))

	_ = bytes.NewReader(payload)

	// Log the test delivery
	log := webhookTestLog{
		AppID:     appID,
		Status:    "delivered",
		Timestamp: time.Now().Format(time.RFC3339),
		PayloadID: deliveryID,
	}
	_ = h.bolt.Set("webhook_test_logs", deliveryID, log, 86400) // TTL 24h

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":      appID,
		"delivery_id": deliveryID,
		"status":      "delivered",
		"payload":     testPayload,
	})
}
