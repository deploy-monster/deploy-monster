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

// ─── Clone App ───────────────────────────────────────────────────────────────

func TestClone_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "source1",
		ProjectID:  "proj1",
		TenantID:   "tenant1",
		Name:       "Original App",
		Type:       "service",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo",
		Branch:     "main",
		Replicas:   2,
		Status:     "running",
	})

	events := core.NewEventBus(nil)
	handler := NewCloneHandler(store, events)

	body, _ := json.Marshal(cloneRequest{NewName: "Cloned App"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/source1/clone", bytes.NewReader(body))
	req.SetPathValue("id", "source1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Clone(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var cloned core.Application
	json.Unmarshal(rr.Body.Bytes(), &cloned)

	if cloned.Name != "Cloned App" {
		t.Errorf("expected name 'Cloned App', got %q", cloned.Name)
	}
	if cloned.ProjectID != "proj1" {
		t.Errorf("expected project_id 'proj1', got %q", cloned.ProjectID)
	}
	if cloned.TenantID != "tenant1" {
		t.Errorf("expected tenant_id 'tenant1', got %q", cloned.TenantID)
	}
	if cloned.Type != "service" {
		t.Errorf("expected type 'service', got %q", cloned.Type)
	}
	if cloned.SourceType != "git" {
		t.Errorf("expected source_type 'git', got %q", cloned.SourceType)
	}
	if cloned.SourceURL != "https://github.com/user/repo" {
		t.Errorf("expected source_url preserved, got %q", cloned.SourceURL)
	}
	if cloned.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", cloned.Branch)
	}
	if cloned.Replicas != 2 {
		t.Errorf("expected replicas 2, got %d", cloned.Replicas)
	}
	if cloned.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", cloned.Status)
	}
	if cloned.ID == "" {
		t.Error("expected non-empty cloned app ID")
	}
	if cloned.ID == "source1" {
		t.Error("cloned app should have a different ID from source")
	}
}

func TestClone_DefaultName(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "source1",
		Name:       "MyApp",
		TenantID:   "tenant1",
		Type:       "service",
		SourceType: "image",
		Status:     "running",
	})

	events := core.NewEventBus(nil)
	handler := NewCloneHandler(store, events)

	// No new_name provided — should default to "MyApp-copy"
	body, _ := json.Marshal(cloneRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/source1/clone", bytes.NewReader(body))
	req.SetPathValue("id", "source1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Clone(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var cloned core.Application
	json.Unmarshal(rr.Body.Bytes(), &cloned)

	if cloned.Name != "MyApp-copy" {
		t.Errorf("expected default name 'MyApp-copy', got %q", cloned.Name)
	}
}

func TestClone_SourceNotFound(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewCloneHandler(store, events)

	body, _ := json.Marshal(cloneRequest{NewName: "Clone"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/nonexistent/clone", bytes.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Clone(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "application not found")
}

func TestClone_InvalidJSON(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewCloneHandler(store, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/source1/clone", bytes.NewReader([]byte("bad")))
	req.SetPathValue("id", "source1")
	rr := httptest.NewRecorder()

	handler.Clone(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestClone_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "source1",
		Name:     "Original",
		TenantID: "tenant1",
		Type:     "service",
		Status:   "running",
	})
	store.errCreateApp = errors.New("db error")

	events := core.NewEventBus(nil)
	handler := NewCloneHandler(store, events)

	body, _ := json.Marshal(cloneRequest{NewName: "Clone"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/source1/clone", bytes.NewReader(body))
	req.SetPathValue("id", "source1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Clone(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "clone failed")
}

func TestClone_GetAppStoreError(t *testing.T) {
	store := newMockStore()
	store.errGetApp = errors.New("db error")

	events := core.NewEventBus(nil)
	handler := NewCloneHandler(store, events)

	body, _ := json.Marshal(cloneRequest{NewName: "Clone"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/source1/clone", bytes.NewReader(body))
	req.SetPathValue("id", "source1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Clone(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
