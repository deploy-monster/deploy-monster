package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Webhook Log List ────────────────────────────────────────────────────────

func TestWebhookLogList_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewWebhookLogHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/webhooks/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
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
	store.addApp(&core.Application{ID: "", TenantID: "t1", Name: "App"})
	handler := NewWebhookLogHandler(store, newMockBoltStore())

	// Empty app ID should be rejected with 400 (path param validation).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps//webhooks/logs", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhookLogList_ResponseFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewWebhookLogHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/webhooks/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestWebhookLogList_TenantScoped(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	bolt := newMockBoltStore()
	bolt.Set(deliveryLogBucket, "log-t1", WebhookDeliveryLog{
		ID: "log-t1", URL: "https://tenant1.example/hook", Status: "sent", Timestamp: 2, TenantID: "t1",
	}, 0)
	bolt.Set(deliveryLogBucket, "log-t2", WebhookDeliveryLog{
		ID: "log-t2", URL: "https://tenant2.example/hook", Status: "sent", Timestamp: 1, TenantID: "t2",
	}, 0)
	bolt.Set(deliveryLogBucket, "legacy", WebhookDeliveryLog{
		ID: "legacy", URL: "https://legacy.example/hook", Status: "sent", Timestamp: 3,
	}, 0)
	handler := NewWebhookLogHandler(store, bolt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/webhooks/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if int(resp["total"].(float64)) != 1 {
		t.Fatalf("expected one tenant-local log, got %v", resp["total"])
	}
	data := resp["data"].([]any)
	item := data[0].(map[string]any)
	if item["id"] != "log-t1" {
		t.Fatalf("listed log id = %v, want log-t1", item["id"])
	}
}
