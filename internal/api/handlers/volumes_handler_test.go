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

// ─── List Volumes ────────────────────────────────────────────────────────────

func TestListVolumes_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Web App"})
	store.addApp(&core.Application{ID: "app2", TenantID: "tenant1", Name: "API Server"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "container1abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "app1",
					"monster.app.name": "Web App",
				},
			},
			{
				ID:    "container2abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "app2",
					"monster.app.name": "API Server",
				},
			},
		},
	}

	handler := NewVolumeHandler(runtime, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

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
		t.Errorf("expected 2 volumes, got %v", resp["data"])
	}
}

func TestListVolumes_NilRuntime(t *testing.T) {
	handler := NewVolumeHandler(nil, newMockStore(), testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
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

func TestListVolumes_RuntimeError(t *testing.T) {
	store := newMockStore()
	runtime := &mockContainerRuntime{
		listErr: errors.New("docker unavailable"),
	}

	handler := NewVolumeHandler(runtime, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to list volumes")
}

func TestListVolumes_DeduplicatesApps(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Web App"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "container1abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "app1",
					"monster.app.name": "Web App",
				},
			},
			{
				ID:    "container2abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "app1", // same app
					"monster.app.name": "Web App",
				},
			},
		},
	}

	handler := NewVolumeHandler(runtime, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 1 {
		t.Errorf("expected total=1 (deduplicated), got %d", total)
	}
}

func TestListVolumes_FiltersCrossTenantApps(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "tenant-app", TenantID: "tenant1", Name: "Visible"})
	store.addApp(&core.Application{ID: "other-app", TenantID: "tenant2", Name: "Hidden"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "tenant-container",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "tenant-app",
					"monster.app.name": "Visible",
				},
			},
			{
				ID: "other-container",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "other-app",
					"monster.app.name": "Hidden",
				},
			},
		},
	}
	handler := NewVolumeHandler(runtime, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(resp["total"].(float64)) != 1 {
		t.Fatalf("expected one visible volume, got %v", resp["total"])
	}
	data := resp["data"].([]any)
	got := data[0].(map[string]any)
	if got["app_id"] != "tenant-app" {
		t.Fatalf("visible app_id = %v, want tenant-app", got["app_id"])
	}
}

// ─── Create Volume ───────────────────────────────────────────────────────────

func TestCreateVolume_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Web App"})
	handler := NewVolumeHandler(nil, store, testCore().Events)

	body, _ := json.Marshal(createVolumeRequest{
		Name:      "my-volume",
		AppID:     "app1",
		MountPath: "/data",
		SizeMB:    512,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["name"] != "my-volume" {
		t.Errorf("expected name=my-volume, got %v", resp["name"])
	}
	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["mount_path"] != "/data" {
		t.Errorf("expected mount_path=/data, got %v", resp["mount_path"])
	}
	if int(resp["size_mb"].(float64)) != 512 {
		t.Errorf("expected size_mb=512, got %v", resp["size_mb"])
	}
	if resp["driver"] != "local" {
		t.Errorf("expected driver=local, got %v", resp["driver"])
	}
	if resp["status"] != "created" {
		t.Errorf("expected status=created, got %v", resp["status"])
	}
}

func TestCreateVolume_InvalidJSON(t *testing.T) {
	handler := NewVolumeHandler(nil, nil, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader([]byte("{")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCreateVolume_RejectsUnknownFields(t *testing.T) {
	handler := NewVolumeHandler(nil, nil, testCore().Events)

	body := []byte(`{"name":"my-volume","extra":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCreateVolume_MissingName(t *testing.T) {
	handler := NewVolumeHandler(nil, nil, testCore().Events)

	body, _ := json.Marshal(createVolumeRequest{
		AppID:     "app1",
		MountPath: "/data",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "name is required")
}

func TestCreateVolume_Unauthorized(t *testing.T) {
	handler := NewVolumeHandler(nil, nil, testCore().Events)

	body, _ := json.Marshal(createVolumeRequest{Name: "my-volume"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestCreateVolume_CrossTenantAppRejected(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant2", Name: "Hidden"})
	handler := NewVolumeHandler(nil, store, testCore().Events)

	body, _ := json.Marshal(createVolumeRequest{
		Name:      "my-volume",
		AppID:     "app1",
		MountPath: "/data",
		SizeMB:    512,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "app not found")
}
