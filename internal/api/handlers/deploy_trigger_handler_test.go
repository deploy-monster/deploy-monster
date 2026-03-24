package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Deploy Trigger (image-based app) ────────────────────────────────────────

func TestDeployTrigger_ImageApp_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.nextDeployVersion["app1"] = 3

	runtime := &mockContainerRuntime{}
	handler := NewDeployTriggerHandler(store, runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "deployed" {
		t.Errorf("expected status=deployed, got %v", resp["status"])
	}

	dep, ok := resp["deployment"].(map[string]any)
	if !ok {
		t.Fatal("expected deployment object in response")
	}
	if dep["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", dep["app_id"])
	}
	if dep["image"] != "nginx:latest" {
		t.Errorf("expected image=nginx:latest, got %v", dep["image"])
	}
	if int(dep["version"].(float64)) != 3 {
		t.Errorf("expected version=3, got %v", dep["version"])
	}
	if dep["triggered_by"] != "manual" {
		t.Errorf("expected triggered_by=manual, got %v", dep["triggered_by"])
	}

	// Verify app status was updated to running.
	if store.updatedStatus["app1"] != "running" {
		t.Errorf("expected app status=running, got %q", store.updatedStatus["app1"])
	}
}

func TestDeployTrigger_ImageApp_NoRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})

	handler := NewDeployTriggerHandler(store, nil, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	// Without runtime, the image deploy still succeeds (just no container created).
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Deploy Trigger (git-based app) ──────────────────────────────────────────

func TestDeployTrigger_GitApp_Accepted(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app2",
		TenantID:   "tenant1",
		Name:       "My Git App",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo.git",
		Branch:     "main",
		Status:     "running",
	})

	runtime := &mockContainerRuntime{}
	handler := NewDeployTriggerHandler(store, runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app2/deploy", nil)
	req.SetPathValue("id", "app2")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "building" {
		t.Errorf("expected status=building, got %q", resp["status"])
	}
	if resp["message"] == "" {
		t.Error("expected non-empty message")
	}
}

// ─── Deploy Trigger — App Not Found ──────────────────────────────────────────

func TestDeployTrigger_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewDeployTriggerHandler(store, nil, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/nonexistent/deploy", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}
