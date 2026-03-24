package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Container Top (Process List) ────────────────────────────────────────────

func TestContainerTop_Success(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "container123456789abcdef",
				Name:   "myapp-abc123",
				Status: "running",
			},
		},
	}

	handler := NewContainerTopHandler(runtime)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/processes", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Top(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["container_id"] != "container123" {
		t.Errorf("expected container_id=container123, got %v", resp["container_id"])
	}

	processes, ok := resp["processes"].([]any)
	if !ok {
		t.Fatal("expected processes array in response")
	}
	if len(processes) != 0 {
		t.Errorf("expected empty processes, got %d items", len(processes))
	}

	titles, ok := resp["titles"].([]any)
	if !ok {
		t.Fatal("expected titles array in response")
	}
	if len(titles) != 4 {
		t.Errorf("expected 4 titles, got %d", len(titles))
	}
}

func TestContainerTop_NoRuntime(t *testing.T) {
	handler := NewContainerTopHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/processes", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Top(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "runtime not available")
}

func TestContainerTop_NoContainer(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{},
	}

	handler := NewContainerTopHandler(runtime)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/processes", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Top(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no container found")
}

func TestContainerTop_RuntimeListError(t *testing.T) {
	runtime := &mockContainerRuntime{
		listErr: errors.New("docker error"),
	}

	handler := NewContainerTopHandler(runtime)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/processes", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Top(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no container found")
}
