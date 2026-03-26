package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Mock Container Runtime ──────────────────────────────────────────────────

type testContainerRuntime struct {
	createErr   error
	containerID string
}

func (r *testContainerRuntime) Ping() error { return nil }

func (r *testContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	if r.createErr != nil {
		return "", r.createErr
	}
	id := r.containerID
	if id == "" {
		id = "container-" + core.GenerateID()[:8]
	}
	return id, nil
}

func (r *testContainerRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (r *testContainerRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (r *testContainerRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (r *testContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (r *testContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}

func (r *testContainerRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}

func (r *testContainerRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}

func (r *testContainerRuntime) ImagePull(_ context.Context, _ string) error { return nil }

func (r *testContainerRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}

func (r *testContainerRuntime) ImageRemove(_ context.Context, _ string) error { return nil }

func (r *testContainerRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}

func (r *testContainerRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// ─── List Engines ────────────────────────────────────────────────────────────

func TestListEngines_Success(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/engines", nil)
	rr := httptest.NewRecorder()

	handler.ListEngines(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	// Registry has postgres, mysql, mariadb, redis, mongodb
	if len(data) < 3 {
		t.Errorf("expected at least 3 engines, got %d", len(data))
	}

	// Verify each engine has required fields.
	for _, item := range data {
		engine := item.(map[string]any)
		if engine["id"] == nil || engine["id"] == "" {
			t.Error("expected non-empty engine id")
		}
		if engine["name"] == nil || engine["name"] == "" {
			t.Error("expected non-empty engine name")
		}
		if engine["versions"] == nil {
			t.Error("expected versions field")
		}
		if engine["default_port"] == nil {
			t.Error("expected default_port field")
		}
	}
}

// ─── Create Database ─────────────────────────────────────────────────────────

func TestCreateDatabase_Success(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{containerID: "pg-container-123"}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	body, _ := json.Marshal(createDBRequest{
		Name:   "mydb",
		Engine: "postgres",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["engine"] != "postgres" {
		t.Errorf("expected engine postgres, got %v", resp["engine"])
	}
	if resp["name"] != "mydb" {
		t.Errorf("expected name mydb, got %v", resp["name"])
	}
	if resp["container_id"] != "pg-container-123" {
		t.Errorf("expected container_id pg-container-123, got %v", resp["container_id"])
	}
	if resp["connection_string"] == nil || resp["connection_string"] == "" {
		t.Error("expected non-empty connection_string")
	}

	creds, ok := resp["credentials"].(map[string]any)
	if !ok {
		t.Fatal("expected credentials object")
	}
	if creds["database"] != "mydb" {
		t.Errorf("expected credentials database=mydb, got %v", creds["database"])
	}
	if creds["user"] == nil || creds["user"] == "" {
		t.Error("expected non-empty credentials user")
	}
	if creds["password"] == nil || creds["password"] == "" {
		t.Error("expected non-empty credentials password")
	}
}

func TestCreateDatabase_MySQL(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	body, _ := json.Marshal(createDBRequest{
		Name:   "app_db",
		Engine: "mysql",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["engine"] != "mysql" {
		t.Errorf("expected engine mysql, got %v", resp["engine"])
	}
}

func TestCreateDatabase_Redis(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	body, _ := json.Marshal(createDBRequest{
		Name:   "cache",
		Engine: "redis",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["engine"] != "redis" {
		t.Errorf("expected engine redis, got %v", resp["engine"])
	}
	port := int(resp["port"].(float64))
	if port != 6379 {
		t.Errorf("expected port 6379, got %d", port)
	}
}

func TestCreateDatabase_NoClaims(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	body, _ := json.Marshal(createDBRequest{Name: "db", Engine: "postgres"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCreateDatabase_InvalidJSON(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader([]byte("bad")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCreateDatabase_MissingFields(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	tests := []struct {
		name string
		body createDBRequest
	}{
		{"missing name", createDBRequest{Engine: "postgres"}},
		{"missing engine", createDBRequest{Name: "db"}},
		{"both empty", createDBRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
			assertErrorMessage(t, rr, "name and engine are required")
		})
	}
}

func TestCreateDatabase_UnsupportedEngine(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	body, _ := json.Marshal(createDBRequest{Name: "db", Engine: "oracle"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unsupported engine: oracle")
}

func TestCreateDatabase_NoRuntime(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	// runtime is nil
	handler := NewDatabaseHandler(store, nil, events)

	body, _ := json.Marshal(createDBRequest{Name: "db", Engine: "postgres"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "container runtime not available")
}

func TestCreateDatabase_ProvisioningError(t *testing.T) {
	store := newMockStore()
	runtime := &testContainerRuntime{createErr: errors.New("docker socket unavailable")}
	events := core.NewEventBus(nil)
	handler := NewDatabaseHandler(store, runtime, events)

	body, _ := json.Marshal(createDBRequest{Name: "db", Engine: "postgres"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
