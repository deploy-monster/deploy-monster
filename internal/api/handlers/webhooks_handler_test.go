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
