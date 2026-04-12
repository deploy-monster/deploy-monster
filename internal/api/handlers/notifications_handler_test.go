package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Notification Test Send ──────────────────────────────────────────────────

func TestNotificationTest_Success(t *testing.T) {
	sender := &mockNotificationSender{}
	handler := NewNotificationHandler(sender)

	body, _ := json.Marshal(testNotificationRequest{
		Channel:   "slack",
		Recipient: "#ops",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "sent" {
		t.Errorf("expected status=sent, got %q", resp["status"])
	}
	if resp["channel"] != "slack" {
		t.Errorf("expected channel=slack, got %q", resp["channel"])
	}

	if sender.lastNotification == nil {
		t.Fatal("expected notification to be sent")
	}
	if sender.lastNotification.Channel != "slack" {
		t.Errorf("expected notification channel=slack, got %q", sender.lastNotification.Channel)
	}
	if sender.lastNotification.Recipient != "#ops" {
		t.Errorf("expected notification recipient=#ops, got %q", sender.lastNotification.Recipient)
	}
}

func TestNotificationTest_InvalidJSON(t *testing.T) {
	sender := &mockNotificationSender{}
	handler := NewNotificationHandler(sender)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", bytes.NewReader([]byte("{")))
	rr := httptest.NewRecorder()

	handler.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestNotificationTest_MissingChannel(t *testing.T) {
	sender := &mockNotificationSender{}
	handler := NewNotificationHandler(sender)

	body, _ := json.Marshal(testNotificationRequest{
		Recipient: "#ops",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "channel is required")
}

func TestNotificationTest_NilSender(t *testing.T) {
	handler := NewNotificationHandler(nil)

	body, _ := json.Marshal(testNotificationRequest{
		Channel:   "slack",
		Recipient: "#ops",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Test(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "notification system not configured")
}

func TestNotificationTest_SendError(t *testing.T) {
	sender := &mockNotificationSender{sendErr: errors.New("connection refused")}
	handler := NewNotificationHandler(sender)

	body, _ := json.Marshal(testNotificationRequest{
		Channel:   "slack",
		Recipient: "#ops",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Test(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["message"] != "notification failed" {
		t.Error("expected sanitized error message 'notification failed'")
	}
}
