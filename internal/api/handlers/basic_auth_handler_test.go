package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Basic Auth ──────────────────────────────────────────────────────────────

func TestBasicAuth_Get_Success(t *testing.T) {
	store := newMockStore()
	handler := NewBasicAuthHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/basic-auth", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp BasicAuthConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Enabled {
		t.Error("expected enabled=false")
	}
	if resp.Realm != "Restricted" {
		t.Errorf("expected realm 'Restricted', got %q", resp.Realm)
	}
}

func TestBasicAuth_Update_Success(t *testing.T) {
	store := newMockStore()
	handler := NewBasicAuthHandler(store, newMockBoltStore())

	body, _ := json.Marshal(BasicAuthConfig{
		Enabled: true,
		Users:   map[string]string{"admin": "$2a$10$hash"},
		Realm:   "Admin Area",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/basic-auth", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}

	cfg, ok := resp["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config object in response")
	}
	if cfg["realm"] != "Admin Area" {
		t.Errorf("expected realm 'Admin Area', got %v", cfg["realm"])
	}
}

func TestBasicAuth_Update_DefaultRealm(t *testing.T) {
	store := newMockStore()
	handler := NewBasicAuthHandler(store, newMockBoltStore())

	body, _ := json.Marshal(BasicAuthConfig{
		Enabled: true,
		Users:   map[string]string{"user": "hash"},
		Realm:   "",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/basic-auth", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	cfg := resp["config"].(map[string]any)
	if cfg["realm"] != "Restricted" {
		t.Errorf("expected default realm 'Restricted', got %v", cfg["realm"])
	}
}

func TestBasicAuth_Update_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewBasicAuthHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/basic-auth", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}
