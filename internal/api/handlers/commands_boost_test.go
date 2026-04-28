package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestCommand_History_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewCommandHandler(&mockContainerRuntime{}, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCommand_History_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewCommandHandler(&mockContainerRuntime{}, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestCommand_History_WrongTenant(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant2", Name: "Test", Status: "running"})
	handler := NewCommandHandler(&mockContainerRuntime{}, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}
