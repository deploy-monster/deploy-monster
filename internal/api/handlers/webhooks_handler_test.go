package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Webhook Log List ────────────────────────────────────────────────────────

func TestWebhookLogList_Success(t *testing.T) {
	store := newMockStore()
	handler := NewWebhookLogHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/webhooks/logs", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	total, ok := resp["total"].(float64)
	if !ok || int(total) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}

	data, ok := resp["data"].([]any)
	if !ok || len(data) != 0 {
		t.Errorf("expected empty data array, got %v", resp["data"])
	}
}

func TestWebhookLogList_EmptyAppID(t *testing.T) {
	store := newMockStore()
	handler := NewWebhookLogHandler(store, newMockBoltStore())

	// Even with no path value, the handler should return 200 with empty data.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps//webhooks/logs", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestWebhookLogList_ResponseFormat(t *testing.T) {
	store := newMockStore()
	handler := NewWebhookLogHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/webhooks/logs", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}
