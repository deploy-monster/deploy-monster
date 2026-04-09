package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── App Pin ─────────────────────────────────────────────────────────────────

func TestAppPin_Pin_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewPinHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/pin", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Pin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %q", resp["app_id"])
	}
	if resp["pinned"] != "true" {
		t.Errorf("expected pinned=true, got %q", resp["pinned"])
	}
}

func TestAppPin_Pin_DifferentAppID(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "my-special-app", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewPinHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/my-special-app/pin", nil)
	req.SetPathValue("id", "my-special-app")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Pin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "my-special-app" {
		t.Errorf("expected app_id=my-special-app, got %q", resp["app_id"])
	}
}

func TestAppPin_Unpin_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewPinHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1/pin", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Unpin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %q", resp["app_id"])
	}
	if resp["pinned"] != "false" {
		t.Errorf("expected pinned=false, got %q", resp["pinned"])
	}
}

func TestAppPin_Unpin_DifferentAppID(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app99", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewPinHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app99/pin", nil)
	req.SetPathValue("id", "app99")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Unpin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app99" {
		t.Errorf("expected app_id=app99, got %q", resp["app_id"])
	}
	if resp["pinned"] != "false" {
		t.Errorf("expected pinned=false, got %q", resp["pinned"])
	}
}
