package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestDeleteDomain_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDeleteDomain_MissingID(t *testing.T) {
	store := newMockStore()
	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestDeleteDomain_GetDomainError(t *testing.T) {
	store := newMockStore()
	store.errGetDomain = errors.New("db error")
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestDeleteDomain_GetAppError(t *testing.T) {
	store := newMockStore()
	store.errGetApp = errors.New("db error")
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestDeleteDomain_WrongTenant(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t2", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
