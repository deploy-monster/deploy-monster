package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── App Logs ────────────────────────────────────────────────────────────────

func TestGetLogs_Success(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "abc123def456",
				Name:   "my-app",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app1"},
			},
		},
		logsData: "2024-01-01 INFO Starting app\n2024-01-01 INFO Listening on :8080\n",
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(runtime, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["container_id"] != "abc123def456" {
		t.Errorf("expected container_id=abc123def456, got %v", resp["container_id"])
	}

	lines, ok := resp["lines"].([]any)
	if !ok {
		t.Fatalf("expected lines array, got %T", resp["lines"])
	}
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	count := int(resp["count"].(float64))
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}
}

func TestGetLogs_CustomTail(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "abc123def456",
				Name:   "my-app",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app1"},
			},
		},
		logsData: "line1\n",
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(runtime, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs?tail=50", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGetLogs_InvalidTailFallsBackToDefault(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "abc123def456",
				Name:   "my-app",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app1"},
			},
		},
		logsData: "line1\n",
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(runtime, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs?tail=abc", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	// Should still succeed, falling back to tail=100.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGetLogs_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(nil, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "container runtime not available")
}

func TestGetLogs_NoContainersFound(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{}, // empty
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(runtime, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no running container found")
}

func TestGetLogs_ListError(t *testing.T) {
	runtime := &mockContainerRuntime{
		listErr: errors.New("docker unavailable"),
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(runtime, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetLogs_LogsReadError(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "abc123def456",
				Name:   "my-app",
				State:  "running",
				Labels: map[string]string{"monster.app.id": "app1"},
			},
		},
		logsErr: errors.New("stream error"),
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewLogHandler(runtime, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/logs", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetLogs(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to read logs")
}
