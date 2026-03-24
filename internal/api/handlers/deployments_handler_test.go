package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── List Deployments By App ─────────────────────────────────────────────────

func TestListDeploymentsByApp_Success(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.addDeployment("app1", core.Deployment{
		ID:      "dep1",
		AppID:   "app1",
		Version: 1,
		Status:  "success",
		Image:   "nginx:latest",
		CreatedAt: now,
	})
	store.addDeployment("app1", core.Deployment{
		ID:      "dep2",
		AppID:   "app1",
		Version: 2,
		Status:  "running",
		Image:   "nginx:1.25",
		CreatedAt: now,
	})

	handler := NewDeploymentHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.ListByApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}

	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 deployments, got %v", resp["data"])
	}
}

func TestListDeploymentsByApp_Empty(t *testing.T) {
	store := newMockStore()
	handler := NewDeploymentHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.ListByApp(rr, req)

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

func TestListDeploymentsByApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListDeploymentsByApp = errors.New("db error")

	handler := NewDeploymentHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.ListByApp(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Get Latest Deployment ───────────────────────────────────────────────────

func TestGetLatestDeployment_Success(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.latestDeployments["app1"] = &core.Deployment{
		ID:        "dep-latest",
		AppID:     "app1",
		Version:   3,
		Status:    "success",
		Image:     "myapp:v3",
		CreatedAt: now,
	}

	handler := NewDeploymentHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/latest", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.GetLatest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var dep core.Deployment
	json.Unmarshal(rr.Body.Bytes(), &dep)

	if dep.ID != "dep-latest" {
		t.Errorf("expected id dep-latest, got %q", dep.ID)
	}
	if dep.Version != 3 {
		t.Errorf("expected version=3, got %d", dep.Version)
	}
	if dep.Status != "success" {
		t.Errorf("expected status=success, got %q", dep.Status)
	}
}

func TestGetLatestDeployment_NotFound(t *testing.T) {
	store := newMockStore()
	handler := NewDeploymentHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/latest", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.GetLatest(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no deployments found")
}

func TestGetLatestDeployment_StoreError(t *testing.T) {
	store := newMockStore()
	store.errGetLatestDeployment = errors.New("db error")

	handler := NewDeploymentHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/latest", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.GetLatest(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
