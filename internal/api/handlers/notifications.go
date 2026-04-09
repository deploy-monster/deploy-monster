package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// NotificationHandler manages notification channel settings.
type NotificationHandler struct {
	sender core.NotificationSender
}

func NewNotificationHandler(sender core.NotificationSender) *NotificationHandler {
	return &NotificationHandler{sender: sender}
}

type testNotificationRequest struct {
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
}

// Test handles POST /api/v1/notifications/test
// Sends a test notification to verify channel configuration.
func (h *NotificationHandler) Test(w http.ResponseWriter, r *http.Request) {
	var req testNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Channel == "" {
		writeError(w, http.StatusBadRequest, "channel is required")
		return
	}

	if h.sender == nil {
		writeError(w, http.StatusServiceUnavailable, "notification system not configured")
		return
	}

	err := h.sender.Send(r.Context(), core.Notification{
		Channel:   req.Channel,
		Recipient: req.Recipient,
		Subject:   "DeployMonster Test Notification",
		Body:      "If you see this, your notification channel is working correctly!",
		Format:    "text",
	})

	if err != nil {
		internalError(w, "notification failed", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "sent",
		"channel": req.Channel,
	})
}
