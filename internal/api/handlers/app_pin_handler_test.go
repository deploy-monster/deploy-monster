package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── App Pin ─────────────────────────────────────────────────────────────────

func TestAppPin_Pin_Success(t *testing.T) {
	store := newMockStore()
	handler := NewPinHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/pin", nil)
	req.SetPathValue("id", "app1")
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
	handler := NewPinHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/my-special-app/pin", nil)
	req.SetPathValue("id", "my-special-app")
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
	handler := NewPinHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1/pin", nil)
	req.SetPathValue("id", "app1")
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
	handler := NewPinHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app99/pin", nil)
	req.SetPathValue("id", "app99")
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
