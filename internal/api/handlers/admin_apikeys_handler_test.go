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
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
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

func TestAdminAPIKeys_List_DoesNotExposeHashes(t *testing.T) {
	bolt := newMockBoltStore()
	handler := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.Generate(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("generate expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr = httptest.NewRecorder()
	handler.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one key, got %#v", resp["data"])
	}
	item, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object key item, got %#v", data[0])
	}
	if _, ok := item["hash"]; ok {
		t.Fatal("list response must not expose API key hashes")
	}
	if item["prefix"] == "" || item["type"] != "platform" || item["created_by"] != "user1" {
		t.Fatalf("unexpected key item: %#v", item)
	}
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

func TestAdminAPIKeys_RequireSuperAdminInHandler(t *testing.T) {
	handler := NewAdminAPIKeyHandler(newMockStore(), newMockBoltStore())

	tests := []struct {
		name   string
		method string
		path   string
		call   func(http.ResponseWriter, *http.Request)
		claims bool
		role   string
		want   int
	}{
		{
			name:   "list without claims",
			method: http.MethodGet,
			path:   "/api/v1/admin/api-keys",
			call:   handler.List,
			want:   http.StatusUnauthorized,
		},
		{
			name:   "generate tenant admin",
			method: http.MethodPost,
			path:   "/api/v1/admin/api-keys",
			call:   handler.Generate,
			claims: true,
			role:   "role_admin",
			want:   http.StatusForbidden,
		},
		{
			name:   "revoke tenant admin",
			method: http.MethodDelete,
			path:   "/api/v1/admin/api-keys/pfx",
			call:   handler.Revoke,
			claims: true,
			role:   "role_admin",
			want:   http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.method == http.MethodDelete {
				req.SetPathValue("prefix", "pfx")
			}
			if tt.claims {
				req = withClaims(req, "user1", "tenant1", tt.role, "user@test.com")
			}
			rr := httptest.NewRecorder()

			tt.call(rr, req)

			if rr.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rr.Code, rr.Body.String())
			}
		})
	}
}
