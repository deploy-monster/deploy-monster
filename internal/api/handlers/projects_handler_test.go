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

// ─── List Projects ───────────────────────────────────────────────────────────

func TestListProjects_Success(t *testing.T) {
	store := newMockStore()
	store.addProject("tenant1", core.Project{ID: "proj1", TenantID: "tenant1", Name: "Project Alpha"})
	store.addProject("tenant1", core.Project{ID: "proj2", TenantID: "tenant1", Name: "Project Beta"})

	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total, ok := resp["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("expected total=2, got %v", resp["total"])
	}

	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 projects, got %v", resp["data"])
	}
}

func TestListProjects_Empty(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
}

func TestListProjects_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestListProjects_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListProjectsByTenant = errors.New("db error")

	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Create Project ──────────────────────────────────────────────────────────

func TestCreateProject_Success(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{
		"name":        "New Project",
		"description": "A test project",
		"environment": "staging",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var project core.Project
	json.Unmarshal(rr.Body.Bytes(), &project)

	if project.Name != "New Project" {
		t.Errorf("expected name 'New Project', got %q", project.Name)
	}
	if project.Environment != "staging" {
		t.Errorf("expected environment staging, got %q", project.Environment)
	}
	if project.TenantID != "tenant1" {
		t.Errorf("expected tenant_id tenant1, got %q", project.TenantID)
	}
	if project.ID == "" {
		t.Error("expected non-empty project ID")
	}
}

func TestCreateProject_DefaultEnvironment(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": "Prod Project"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var project core.Project
	json.Unmarshal(rr.Body.Bytes(), &project)

	if project.Environment != "production" {
		t.Errorf("expected default environment 'production', got %q", project.Environment)
	}
}

func TestCreateProject_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": "Test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCreateProject_MissingName(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"description": "no name"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "name is required")
}

func TestCreateProject_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader([]byte("{")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCreateProject_StoreError(t *testing.T) {
	store := newMockStore()
	store.errCreateProject = errors.New("db error")

	handler := NewProjectHandler(store, testCore().Events)

	body, _ := json.Marshal(map[string]string{"name": "Fail Project"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Get Project ─────────────────────────────────────────────────────────────

func TestGetProject_Success(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{
		ID:          "proj1",
		TenantID:    "tenant1",
		Name:        "Alpha",
		Environment: "production",
	})

	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var project core.Project
	json.Unmarshal(rr.Body.Bytes(), &project)

	if project.ID != "proj1" {
		t.Errorf("expected id proj1, got %q", project.ID)
	}
	if project.Name != "Alpha" {
		t.Errorf("expected name Alpha, got %q", project.Name)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "project not found")
}

func TestGetProject_StoreError(t *testing.T) {
	store := newMockStore()
	store.errGetProject = errors.New("db error")

	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ─── Delete Project ──────────────────────────────────────────────────────────

func TestDeleteProject_Success(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj1", TenantID: "tenant1", Name: "Doomed"})

	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.deletedProjectID != "proj1" {
		t.Errorf("expected deleted project ID proj1, got %q", store.deletedProjectID)
	}
}

func TestDeleteProject_StoreError(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj1", TenantID: "tenant1", Name: "Test"})
	store.errDeleteProject = errors.New("constraint violation")

	handler := NewProjectHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Integration: Create then Get ────────────────────────────────────────────

func TestProjectCreateThenGet_Integration(t *testing.T) {
	store := newMockStore()
	handler := NewProjectHandler(store, testCore().Events)

	// Create
	body, _ := json.Marshal(map[string]string{"name": "Integrated", "environment": "staging"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	createReq = withClaims(createReq, "user1", "tenant1", "role_owner", "user@example.com")
	createRR := httptest.NewRecorder()
	handler.Create(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created core.Project
	json.Unmarshal(createRR.Body.Bytes(), &created)

	// Get
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+created.ID, nil)
	getReq.SetPathValue("id", created.ID)
	getReq = withClaims(getReq, "user1", "tenant1", "role_owner", "user@example.com")
	getRR := httptest.NewRecorder()
	handler.Get(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", getRR.Code)
	}

	var fetched core.Project
	json.Unmarshal(getRR.Body.Bytes(), &fetched)

	if fetched.ID != created.ID {
		t.Errorf("expected same ID, got %q vs %q", fetched.ID, created.ID)
	}
	if fetched.Name != "Integrated" {
		t.Errorf("expected name 'Integrated', got %q", fetched.Name)
	}
}
