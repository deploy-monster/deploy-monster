package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Deploy Preview ──────────────────────────────────────────────────────────

func TestDeployPreview_Success_Git(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "t1",
		Name:       "Web App",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo",
		Branch:     "main",
		Status:     "running",
	})
	store.nextDeployVersion["app1"] = 3

	handler := NewDeployPreviewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/preview", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id 'app1', got %v", resp["app_id"])
	}
	if resp["app_name"] != "Web App" {
		t.Errorf("expected app_name 'Web App', got %v", resp["app_name"])
	}
	if resp["source_type"] != "git" {
		t.Errorf("expected source_type 'git', got %v", resp["source_type"])
	}
	if resp["branch"] != "main" {
		t.Errorf("expected branch 'main', got %v", resp["branch"])
	}
	if resp["detected_type"] != "would clone and auto-detect" {
		t.Errorf("expected detected_type for git, got %v", resp["detected_type"])
	}
	if resp["dockerfile"] != "auto-generated based on project type" {
		t.Errorf("expected dockerfile description for git, got %v", resp["dockerfile"])
	}
	if int(resp["next_version"].(float64)) != 3 {
		t.Errorf("expected next_version=3, got %v", resp["next_version"])
	}
	if resp["dry_run"] != true {
		t.Errorf("expected dry_run=true, got %v", resp["dry_run"])
	}
	if resp["strategy"] != "recreate" {
		t.Errorf("expected strategy 'recreate', got %v", resp["strategy"])
	}
}

func TestDeployPreview_Success_Image(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app2",
		TenantID:   "t1",
		Name:       "Docker App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})

	handler := NewDeployPreviewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app2/deploy/preview", nil)
	req.SetPathValue("id", "app2")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["detected_type"] != "image pull: nginx:latest" {
		t.Errorf("expected image pull detected_type, got %v", resp["detected_type"])
	}
}

func TestDeployPreview_WithExistingDeployment(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "t1",
		Name:       "Web App",
		SourceType: "git",
		Status:     "running",
	})
	store.latestDeployments["app1"] = &core.Deployment{
		ID:      "dep1",
		AppID:   "app1",
		Version: 5,
		Image:   "myapp:v5",
	}
	store.nextDeployVersion["app1"] = 6

	handler := NewDeployPreviewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/preview", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	currentVersion := int(resp["current_version"].(float64))
	if currentVersion != 5 {
		t.Errorf("expected current_version=5, got %d", currentVersion)
	}
	if resp["current_image"] != "myapp:v5" {
		t.Errorf("expected current_image 'myapp:v5', got %v", resp["current_image"])
	}
	nextVersion := int(resp["next_version"].(float64))
	if nextVersion != 6 {
		t.Errorf("expected next_version=6, got %d", nextVersion)
	}
}

func TestDeployPreview_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewDeployPreviewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/nonexistent/deploy/preview", nil)
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Preview(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "application not found")
}

func TestDeployPreview_SupportedTypes(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "t1",
		Name:       "App",
		SourceType: "git",
		Status:     "running",
	})

	handler := NewDeployPreviewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/preview", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	types, ok := resp["supported_types"].([]any)
	if !ok {
		t.Fatal("expected supported_types array")
	}
	if len(types) == 0 {
		t.Error("expected non-empty supported_types")
	}
}

func TestDeployPreview_CustomDockerfile(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "t1",
		Name:       "Custom App",
		SourceType: "dockerfile",
		Dockerfile: "Dockerfile.prod",
		Status:     "running",
	})

	handler := NewDeployPreviewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/preview", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["dockerfile"] != "custom: Dockerfile.prod" {
		t.Errorf("expected dockerfile 'custom: Dockerfile.prod', got %v", resp["dockerfile"])
	}
}
