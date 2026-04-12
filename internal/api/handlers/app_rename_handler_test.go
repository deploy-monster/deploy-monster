package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Rename App ──────────────────────────────────────────────────────────────

func TestRename_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Name:     "Old Name",
		Status:   "running",
	})

	handler := NewRenameHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rename", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Rename(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %q", resp["app_id"])
	}
	if resp["old_name"] != "Old Name" {
		t.Errorf("expected old_name=Old Name, got %q", resp["old_name"])
	}
	if resp["new_name"] != "New Name" {
		t.Errorf("expected new_name=New Name, got %q", resp["new_name"])
	}

	// Verify store was updated.
	if store.updatedApp == nil {
		t.Fatal("expected app to be updated in store")
	}
	if store.updatedApp.Name != "New Name" {
		t.Errorf("expected stored name=New Name, got %q", store.updatedApp.Name)
	}
}

func TestRename_EmptyName(t *testing.T) {
	store := newMockStore()
	handler := NewRenameHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rename", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Rename(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "name is required")
}

func TestRename_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewRenameHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rename", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Rename(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestRename_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewRenameHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/nonexistent/rename", bytes.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Rename(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "application not found")
}

func TestRename_UpdateError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Name:     "Old Name",
		Status:   "running",
	})
	store.errUpdateApp = errors.New("db error")

	handler := NewRenameHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rename", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Rename(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "rename failed")
}
