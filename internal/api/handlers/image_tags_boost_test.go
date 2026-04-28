package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestImageTagHandler_List_Unauthorized(t *testing.T) {
	store := newMockStore()
	h := NewImageTagHandler(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	// No claims
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_ForbiddenImage(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	h := NewImageTagHandler(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=redis", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_RuntimeError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	runtime := &mockContainerRuntime{imageListErr: context.Canceled}
	h := NewImageTagHandler(store, runtime)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_EmptyResult(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	runtime := &mockContainerRuntime{imageList: []core.ImageInfo{}}
	h := NewImageTagHandler(store, runtime)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	tags, _ := resp["tags"].([]any)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
	if resp["total"] != float64(0) {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

func TestImageTagHandler_List_WithMatches_Allowed(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	runtime := &mockContainerRuntime{
		imageList: []core.ImageInfo{
			{ID: "sha256:abc", Tags: []string{"nginx:latest", "nginx:1.25"}, Size: 1000000},
			{ID: "sha256:def", Tags: []string{"redis:7"}, Size: 500000},
		},
	}
	h := NewImageTagHandler(store, runtime)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["image"] != "nginx" {
		t.Errorf("image = %v, want nginx", resp["image"])
	}
	tags, _ := resp["tags"].([]any)
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}
