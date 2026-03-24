package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Activity Feed ───────────────────────────────────────────────────────────

func TestActivityFeed_Success(t *testing.T) {
	store := newMockStore()
	store.addAuditLog("tenant1", core.AuditEntry{
		ID:           1,
		TenantID:     "tenant1",
		UserID:       "user1",
		Action:       "app.created",
		ResourceType: "application",
		ResourceID:   "app1",
	})
	store.addAuditLog("tenant1", core.AuditEntry{
		ID:           2,
		TenantID:     "tenant1",
		UserID:       "user1",
		Action:       "app.deployed",
		ResourceType: "application",
		ResourceID:   "app2",
	})

	handler := NewActivityHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total, ok := resp["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("expected total=2, got %v", resp["total"])
	}

	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 entries in data, got %v", resp["data"])
	}
}

func TestActivityFeed_CustomLimit(t *testing.T) {
	store := newMockStore()
	store.addAuditLog("tenant1", core.AuditEntry{ID: 1, TenantID: "tenant1"})

	handler := NewActivityHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=5", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestActivityFeed_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewActivityHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestActivityFeed_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListAuditLogs = errors.New("db error")

	handler := NewActivityHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestActivityFeed_EmptyResult(t *testing.T) {
	store := newMockStore()
	handler := NewActivityHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
}

func TestActivityFeed_InvalidLimit(t *testing.T) {
	store := newMockStore()
	handler := NewActivityHandler(store)

	// Negative limit should default to 20
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=-1", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestActivityFeed_LimitOverMax(t *testing.T) {
	store := newMockStore()
	handler := NewActivityHandler(store)

	// Limit > 100 should default to 20
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=200", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Feed(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
