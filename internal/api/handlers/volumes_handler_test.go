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

	handler := NewVolumeHandler(runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
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
	handler := NewVolumeHandler(nil, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
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
	runtime := &mockContainerRuntime{
		listErr: errors.New("docker unavailable"),
	}

	handler := NewVolumeHandler(runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to list volumes")
}

func TestListVolumes_DeduplicatesApps(t *testing.T) {
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

	handler := NewVolumeHandler(runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/volumes", nil)
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

// ─── Create Volume ───────────────────────────────────────────────────────────

func TestCreateVolume_Success(t *testing.T) {
	handler := NewVolumeHandler(nil, testCore().Events)

	body, _ := json.Marshal(createVolumeRequest{
		Name:      "my-volume",
		AppID:     "app1",
		MountPath: "/data",
		SizeMB:    512,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
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
	handler := NewVolumeHandler(nil, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader([]byte("{")))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCreateVolume_MissingName(t *testing.T) {
	handler := NewVolumeHandler(nil, testCore().Events)

	body, _ := json.Marshal(createVolumeRequest{
		AppID:     "app1",
		MountPath: "/data",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/volumes", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "name is required")
}
