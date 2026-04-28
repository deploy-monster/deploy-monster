package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestRequireTenantProject_NoClaims(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	w := httptest.NewRecorder()

	store := newMockStore()
	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
}

func TestRequireTenantProject_MissingID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/projects/", nil)
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	store := newMockStore()
	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
}

func TestRequireTenantProject_NotFound(t *testing.T) {
	store := newMockStore()

	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", w.Code)
	}
}

func TestRequireTenantProject_WrongTenant(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj1", TenantID: "tenant2", Name: "Alpha"})

	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", w.Code)
	}
}

func TestRequireTenantProject_Success(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj1", TenantID: "tenant1", Name: "Alpha"})

	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	result := requireTenantProject(w, req, store)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "proj1" {
		t.Errorf("id = %q, want proj1", result.ID)
	}
}
