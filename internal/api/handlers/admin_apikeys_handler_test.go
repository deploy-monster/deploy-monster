package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── List API Keys ───────────────────────────────────────────────────────────

func TestAdminAPIKeys_List_Success(t *testing.T) {
	store := newMockStore()
	handler := NewAdminAPIKeyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

// ─── Generate API Key ────────────────────────────────────────────────────────

func TestAdminAPIKeys_Generate_Success(t *testing.T) {
	store := newMockStore()
	handler := NewAdminAPIKeyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	key, ok := resp["key"].(string)
	if !ok || key == "" {
		t.Error("expected non-empty key in response")
	}
	prefix, ok := resp["prefix"].(string)
	if !ok || prefix == "" {
		t.Error("expected non-empty prefix in response")
	}
	if resp["type"] != "platform" {
		t.Errorf("expected type=platform, got %v", resp["type"])
	}
	if resp["message"] == nil || resp["message"] == "" {
		t.Error("expected non-empty message warning to save key")
	}
}

func TestAdminAPIKeys_Generate_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewAdminAPIKeyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	// No claims.
	rr := httptest.NewRecorder()

	handler.Generate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "super admin required")
}

func TestAdminAPIKeys_Generate_NonAdmin(t *testing.T) {
	store := newMockStore()
	handler := NewAdminAPIKeyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_member", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Generate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "super admin required")
}

// ─── Revoke API Key ──────────────────────────────────────────────────────────

func TestAdminAPIKeys_Revoke_Success(t *testing.T) {
	store := newMockStore()
	handler := NewAdminAPIKeyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/api-keys/dm_abc12345", nil)
	req.SetPathValue("prefix", "dm_abc12345")
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Revoke(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}
