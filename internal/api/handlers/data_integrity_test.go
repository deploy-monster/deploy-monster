package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Duplicate App Name Detection ──────────────────────────────────────────

func TestCreateApp_DuplicateName_Returns409(t *testing.T) {
	store := newMockStore()
	store.apps["existing"] = &core.Application{
		ID: "existing", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	c := &core.Core{
		Config: &core.Config{},
		Events: core.NewEventBus(nil),
	}
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:      "my-app",
		SourceURL: "https://github.com/example/repo",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "already exists") {
		t.Errorf("expected 'already exists' in message, got %q", msg)
	}
}

func TestCreateApp_UniqueNameInDifferentTenant_Succeeds(t *testing.T) {
	store := newMockStore()
	store.apps["existing"] = &core.Application{
		ID: "existing", TenantID: "tenant-other", Name: "my-app", Status: "running",
	}
	c := &core.Core{
		Config: &core.Config{},
		Events: core.NewEventBus(nil),
	}
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:      "my-app",
		SourceURL: "https://github.com/example/repo",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Env Var Payload Size Validation ───────────────────────────────────────

func TestUpdateEnv_KeyTooLong(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID: "app-1", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: strings.Repeat("K", 257), Value: "val"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app-1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "key exceeds") {
		t.Errorf("expected key length error, got %s", rr.Body.String())
	}
}

func TestUpdateEnv_ValueTooLong(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID: "app-1", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: "MY_VAR", Value: strings.Repeat("x", 64*1024+1)},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app-1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "64KB") {
		t.Errorf("expected value size error, got %s", rr.Body.String())
	}
}

func TestUpdateEnv_TotalPayloadTooLarge(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID: "app-1", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	handler := NewEnvVarHandler(store)

	// Create many vars that individually are fine but total > 512KB
	var vars []envVarEntry
	for i := 0; i < 20; i++ {
		vars = append(vars, envVarEntry{Key: "VAR_" + strings.Repeat("K", 10), Value: strings.Repeat("x", 30*1024)})
	}

	body, _ := json.Marshal(map[string]any{"vars": vars})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app-1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "512KB") {
		t.Errorf("expected total payload error, got %s", rr.Body.String())
	}
}

func TestUpdateEnv_ValidPayload_Succeeds(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID: "app-1", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: "DB_HOST", Value: "localhost"},
			{Key: "DB_PORT", Value: "5432"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app-1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
