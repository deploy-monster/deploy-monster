package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── EnvVar Get ──────────────────────────────────────────────────────────────

func TestEnvVarGet_Success(t *testing.T) {
	store := newMockStore()
	envJSON := `[{"key":"DB_HOST","value":"localhost"},{"key":"API_KEY","value":"secret123"}]`
	store.addApp(&core.Application{
		ID:         "app1",
		Name:       "App",
		EnvVarsEnc: envJSON,
	})

	handler := NewEnvVarHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/env", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(data))
	}

	// Values should be masked
	first := data[0].(map[string]any)
	if first["key"] != "DB_HOST" {
		t.Errorf("expected key 'DB_HOST', got %v", first["key"])
	}
	// "localhost" is 9 chars, should be masked as "lo*****st"
	val := first["value"].(string)
	if !strings.Contains(val, "*") {
		t.Errorf("expected masked value, got %q", val)
	}
}

func TestEnvVarGet_SecretReference(t *testing.T) {
	store := newMockStore()
	envJSON := `[{"key":"DB_PASS","value":"${SECRET:db_password}"}]`
	store.addApp(&core.Application{
		ID:         "app1",
		Name:       "App",
		EnvVarsEnc: envJSON,
	})

	handler := NewEnvVarHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/env", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	entry := data[0].(map[string]any)
	// Secret references should NOT be masked
	if entry["value"] != "${SECRET:db_password}" {
		t.Errorf("expected unmasked secret reference, got %v", entry["value"])
	}
}

func TestEnvVarGet_EmptyEnv(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App"})

	handler := NewEnvVarHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/env", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
}

func TestEnvVarGet_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewEnvVarHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent/env", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

// ─── EnvVar Update ───────────────────────────────────────────────────────────

func TestEnvVarUpdate_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App"})

	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: "DB_HOST", Value: "postgres.local"},
			{Key: "DB_PORT", Value: "5432"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "updated" {
		t.Errorf("expected status 'updated', got %q", resp["status"])
	}

	// Verify the app was updated in the store
	if store.updatedApp == nil {
		t.Fatal("expected app to be updated in store")
	}
	if store.updatedApp.EnvVarsEnc == "" {
		t.Error("expected EnvVarsEnc to be set")
	}
}

func TestEnvVarUpdate_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewEnvVarHandler(store)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/env", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestEnvVarUpdate_EmptyKey(t *testing.T) {
	store := newMockStore()
	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: "", Value: "value"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "empty key not allowed")
}

func TestEnvVarUpdate_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: "KEY", Value: "val"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/nonexistent/env", bytes.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

func TestEnvVarUpdate_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App"})
	store.errUpdateApp = errors.New("db error")

	handler := NewEnvVarHandler(store)

	body, _ := json.Marshal(map[string]any{
		"vars": []envVarEntry{
			{Key: "KEY", Value: "val"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/env", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to update env vars")
}

// ─── Env Import ──────────────────────────────────────────────────────────────

func TestEnvImport_DotEnvFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App"})

	handler := NewEnvImportHandler(store)

	dotenv := "DB_HOST=localhost\nDB_PORT=5432\n# comment\nAPI_KEY=\"my-secret\"\n"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", strings.NewReader(dotenv))
	req.SetPathValue("id", "app1")
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()

	handler.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	imported := int(resp["imported"].(float64))
	if imported != 3 {
		t.Errorf("expected imported=3, got %d", imported)
	}
	if resp["status"] != "imported" {
		t.Errorf("expected status 'imported', got %v", resp["status"])
	}
}

func TestEnvImport_JSONFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App"})

	handler := NewEnvImportHandler(store)

	vars, _ := json.Marshal([]envVarEntry{
		{Key: "KEY1", Value: "val1"},
		{Key: "KEY2", Value: "val2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", bytes.NewReader(vars))
	req.SetPathValue("id", "app1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	imported := int(resp["imported"].(float64))
	if imported != 2 {
		t.Errorf("expected imported=2, got %d", imported)
	}
}

func TestEnvImport_EmptyVars(t *testing.T) {
	store := newMockStore()
	handler := NewEnvImportHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", strings.NewReader("# only comments\n"))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no variables found")
}

func TestEnvImport_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewEnvImportHandler(store)

	dotenv := "KEY=value\n"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/nonexistent/env/import", strings.NewReader(dotenv))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Import(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

func TestEnvImport_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewEnvImportHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", strings.NewReader("[bad"))
	req.SetPathValue("id", "app1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid JSON array")
}

// ─── Env Export ──────────────────────────────────────────────────────────────

func TestEnvExport_DotEnvFormat(t *testing.T) {
	store := newMockStore()
	envJSON := `[{"key":"DB_HOST","value":"localhost"},{"key":"DB_PORT","value":"5432"}]`
	store.addApp(&core.Application{
		ID:         "app1",
		Name:       "App",
		EnvVarsEnc: envJSON,
	})

	handler := NewEnvImportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/env/export", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got %q", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "DB_HOST=localhost") {
		t.Errorf("expected DB_HOST=localhost in body, got %q", body)
	}
	if !strings.Contains(body, "DB_PORT=5432") {
		t.Errorf("expected DB_PORT=5432 in body, got %q", body)
	}
}

func TestEnvExport_JSONFormat(t *testing.T) {
	store := newMockStore()
	envJSON := `[{"key":"KEY1","value":"val1"}]`
	store.addApp(&core.Application{
		ID:         "app1",
		Name:       "App",
		EnvVarsEnc: envJSON,
	})

	handler := NewEnvImportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/env/export?format=json", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var vars []envVarEntry
	json.Unmarshal(rr.Body.Bytes(), &vars)

	if len(vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(vars))
	}
	if vars[0].Key != "KEY1" {
		t.Errorf("expected key 'KEY1', got %q", vars[0].Key)
	}
}

func TestEnvExport_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewEnvImportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent/env/export", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Export(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ─── Env Compare ─────────────────────────────────────────────────────────────

func TestEnvCompare_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		Name:       "App 1",
		EnvVarsEnc: `[{"key":"DB_HOST","value":"localhost"},{"key":"APP_PORT","value":"3000"}]`,
	})
	store.addApp(&core.Application{
		ID:         "app2",
		Name:       "App 2",
		EnvVarsEnc: `[{"key":"DB_HOST","value":"remote.db"},{"key":"LOG_LEVEL","value":"debug"}]`,
	})

	handler := NewEnvCompareHandler(store)

	body, _ := json.Marshal(map[string]string{
		"left_app_id":  "app1",
		"right_app_id": "app2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/env/compare", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Compare(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["left"] != "app1" {
		t.Errorf("expected left 'app1', got %v", resp["left"])
	}
	if resp["right"] != "app2" {
		t.Errorf("expected right 'app2', got %v", resp["right"])
	}

	total := int(resp["total"].(float64))
	if total != 3 {
		t.Errorf("expected total=3 diffs (changed DB_HOST, removed APP_PORT, added LOG_LEVEL), got %d", total)
	}

	diffs := resp["diffs"].([]any)
	statuses := map[string]bool{}
	for _, d := range diffs {
		diff := d.(map[string]any)
		statuses[diff["status"].(string)] = true
	}
	if !statuses["changed"] {
		t.Error("expected a 'changed' diff")
	}
	if !statuses["removed"] {
		t.Error("expected a 'removed' diff")
	}
	if !statuses["added"] {
		t.Error("expected an 'added' diff")
	}
}

func TestEnvCompare_LeftAppNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app2", Name: "App 2"})

	handler := NewEnvCompareHandler(store)

	body, _ := json.Marshal(map[string]string{
		"left_app_id":  "nonexistent",
		"right_app_id": "app2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/env/compare", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Compare(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "left app not found")
}

func TestEnvCompare_RightAppNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "App 1"})

	handler := NewEnvCompareHandler(store)

	body, _ := json.Marshal(map[string]string{
		"left_app_id":  "app1",
		"right_app_id": "nonexistent",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/env/compare", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Compare(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "right app not found")
}

func TestEnvCompare_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewEnvCompareHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/env/compare", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()

	handler.Compare(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestEnvCompare_IdenticalApps(t *testing.T) {
	store := newMockStore()
	envJSON := `[{"key":"KEY1","value":"val1"}]`
	store.addApp(&core.Application{ID: "app1", Name: "App 1", EnvVarsEnc: envJSON})
	store.addApp(&core.Application{ID: "app2", Name: "App 2", EnvVarsEnc: envJSON})

	handler := NewEnvCompareHandler(store)

	body, _ := json.Marshal(map[string]string{
		"left_app_id":  "app1",
		"right_app_id": "app2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/env/compare", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Compare(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 0 {
		t.Errorf("expected total=0 (identical envs), got %d", total)
	}
}

// ─── Environment Presets ─────────────────────────────────────────────────────

func TestEnvironmentListPresets_Success(t *testing.T) {
	store := newMockStore()
	handler := NewEnvironmentHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/environments/presets", nil)
	rr := httptest.NewRecorder()

	handler.ListPresets(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 3 {
		t.Errorf("expected 3 presets (production, staging, development), got %d", len(data))
	}
}

func TestEnvironmentApplyPreset_Success(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{
		ID:       "proj1",
		TenantID: "tenant1",
		Name:     "Test Project",
	})

	handler := NewEnvironmentHandler(store)

	body, _ := json.Marshal(map[string]string{"environment": "staging"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj1/environment", bytes.NewReader(body))
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ApplyPreset(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["environment"] != "staging" {
		t.Errorf("expected environment 'staging', got %v", resp["environment"])
	}
	if resp["project_id"] != "proj1" {
		t.Errorf("expected project_id 'proj1', got %v", resp["project_id"])
	}
}

func TestEnvironmentApplyPreset_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewEnvironmentHandler(store)

	body, _ := json.Marshal(map[string]string{"environment": "production"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj1/environment", bytes.NewReader(body))
	req.SetPathValue("id", "proj1")
	rr := httptest.NewRecorder()

	handler.ApplyPreset(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestEnvironmentApplyPreset_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewEnvironmentHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj1/environment", bytes.NewReader([]byte("bad")))
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ApplyPreset(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestEnvironmentApplyPreset_ProjectNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewEnvironmentHandler(store)

	body, _ := json.Marshal(map[string]string{"environment": "production"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/nonexistent/environment", bytes.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ApplyPreset(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "project not found")
}

// ─── maskValue unit tests ────────────────────────────────────────────────────

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ab", "****"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"secret123", "se*****23"},
		{"${SECRET:name}", "${SECRET:name}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := maskValue(tt.input)
			if got != tt.expected {
				t.Errorf("maskValue(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
