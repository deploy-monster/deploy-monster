package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestSnapshot_Create_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/snapshots", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestSnapshot_Create_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app123456789", TenantID: "tenant1", Name: "Test"})
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app123456789/snapshots", nil)
	req.SetPathValue("id", "app123456789")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SnapshotInfo
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.AppID != "app123456789" {
		t.Errorf("app_id = %q, want app1", resp.AppID)
	}
	if resp.ID == "" {
		t.Error("expected non-empty snapshot ID")
	}
}

func TestSnapshot_List_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/snapshots", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestSnapshot_List_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/snapshots", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestSnapshot_List_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app123456789", TenantID: "tenant1", Name: "Test"})
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app123456789/snapshots", nil)
	req.SetPathValue("id", "app123456789")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}
