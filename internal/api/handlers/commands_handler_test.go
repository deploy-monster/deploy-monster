package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Run Command ─────────────────────────────────────────────────────────────

func TestCommand_Run_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "container123456789abcdef",
				Name:   "myapp-abc123",
				Status: "running",
			},
		},
	}

	handler := NewCommandHandler(runtime, store, testCore().Events)

	body, _ := json.Marshal(runCommandRequest{
		Command: "php artisan migrate",
		Timeout: 120,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/commands", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Run(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["command"] != "php artisan migrate" {
		t.Errorf("expected command='php artisan migrate', got %v", resp["command"])
	}
	if int(resp["timeout"].(float64)) != 120 {
		t.Errorf("expected timeout=120, got %v", resp["timeout"])
	}
	if resp["status"] != "queued" {
		t.Errorf("expected status=queued, got %v", resp["status"])
	}
	if resp["container_id"] != "container123" {
		t.Errorf("expected container_id=container123, got %v", resp["container_id"])
	}
}

func TestCommand_Run_DefaultTimeout(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456789abcdef", Name: "myapp", Status: "running"},
		},
	}

	handler := NewCommandHandler(runtime, store, testCore().Events)

	body, _ := json.Marshal(runCommandRequest{Command: "echo hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/commands", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Run(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if int(resp["timeout"].(float64)) != 60 {
		t.Errorf("expected default timeout=60, got %v", resp["timeout"])
	}
}

func TestCommand_Run_EmptyCommand(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewCommandHandler(runtime, store, testCore().Events)

	body, _ := json.Marshal(runCommandRequest{Command: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/commands", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Run(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "command is required")
}

func TestCommand_Run_InvalidJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewCommandHandler(runtime, store, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/commands", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Run(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCommand_Run_NoRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	handler := NewCommandHandler(nil, store, testCore().Events)

	body, _ := json.Marshal(runCommandRequest{Command: "echo hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/commands", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Run(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "container runtime not available")
}

func TestCommand_Run_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{},
	}
	handler := NewCommandHandler(runtime, store, testCore().Events)

	body, _ := json.Marshal(runCommandRequest{Command: "echo hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/commands", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Run(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no running container for app")
}

// ─── Command History ─────────────────────────────────────────────────────────

func TestCommand_History_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{}
	handler := NewCommandHandler(runtime, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
}
