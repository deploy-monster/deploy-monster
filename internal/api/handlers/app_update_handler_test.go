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

// ─── Update App Fields ───────────────────────────────────────────────────────

func TestAppUpdate_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "Original Name",
		SourceURL:  "https://github.com/old/repo.git",
		Branch:     "main",
		Dockerfile: "Dockerfile",
		Replicas:   1,
		Status:     "running",
	})

	c := testCore()
	handler := NewAppHandler(store, c)

	replicas := 3
	body, _ := json.Marshal(updateAppRequest{
		Name:       "New Name",
		SourceURL:  "https://github.com/new/repo.git",
		Branch:     "develop",
		Dockerfile: "Dockerfile.prod",
		Replicas:   &replicas,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app1", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var app core.Application
	json.Unmarshal(rr.Body.Bytes(), &app)

	if app.Name != "New Name" {
		t.Errorf("expected name=New Name, got %q", app.Name)
	}
	if app.SourceURL != "https://github.com/new/repo.git" {
		t.Errorf("expected source_url updated, got %q", app.SourceURL)
	}
	if app.Branch != "develop" {
		t.Errorf("expected branch=develop, got %q", app.Branch)
	}
	if app.Dockerfile != "Dockerfile.prod" {
		t.Errorf("expected dockerfile=Dockerfile.prod, got %q", app.Dockerfile)
	}
	if app.Replicas != 3 {
		t.Errorf("expected replicas=3, got %d", app.Replicas)
	}

	// Verify store was updated.
	if store.updatedApp == nil {
		t.Fatal("expected app to be updated in store")
	}
	if store.updatedApp.Name != "New Name" {
		t.Errorf("expected stored name=New Name, got %q", store.updatedApp.Name)
	}
}

func TestAppUpdate_PartialUpdate(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		Name:     "Original",
		Branch:   "main",
		Replicas: 1,
		Status:   "running",
	})

	c := testCore()
	handler := NewAppHandler(store, c)

	// Only update the branch, leave other fields untouched.
	body, _ := json.Marshal(updateAppRequest{Branch: "staging"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app1", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var app core.Application
	json.Unmarshal(rr.Body.Bytes(), &app)

	if app.Name != "Original" {
		t.Errorf("expected name unchanged=Original, got %q", app.Name)
	}
	if app.Branch != "staging" {
		t.Errorf("expected branch=staging, got %q", app.Branch)
	}
	if app.Replicas != 1 {
		t.Errorf("expected replicas unchanged=1, got %d", app.Replicas)
	}
}

func TestAppUpdate_AppNotFound(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(updateAppRequest{Name: "New Name"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/nonexistent", bytes.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

func TestAppUpdate_InvalidJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App", Status: "running"})

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app1", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestAppUpdate_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App", Status: "running"})
	store.errUpdateApp = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(updateAppRequest{Name: "New"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app1", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "update failed")
}

func TestAppUpdate_GetAppStoreError(t *testing.T) {
	store := newMockStore()
	store.errGetApp = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(updateAppRequest{Name: "New"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app1", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
