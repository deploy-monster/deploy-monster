package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Error Sanitization Tests ───────────────────────────────────────────────

func TestInternalError_DoesNotLeakDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	internalError(rr, "operation failed", fmt.Errorf("secret DB connection string: host=10.0.0.5"))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}

	body := rr.Body.String()
	if strings.Contains(body, "10.0.0.5") {
		t.Error("internal error leaked DB connection details to client")
	}
	if strings.Contains(body, "secret") {
		t.Error("internal error leaked sensitive info to client")
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["message"] != "operation failed" {
		t.Errorf("expected sanitized message, got %v", errObj["message"])
	}
	if errObj["code"] != "internal_error" {
		t.Errorf("expected code=internal_error, got %v", errObj["code"])
	}
}

// ─── App Creation Field Validation ──────────────────────────────────────────

func TestCreateApp_FieldLengthValidation(t *testing.T) {
	store := newMockStore()
	c := &core.Core{
		Config: &core.Config{},
		Events: core.NewEventBus(nil),
	}
	handler := NewAppHandler(store, c)

	tests := []struct {
		name    string
		field   string
		body    createAppRequest
		wantErr string
	}{
		{
			name:  "source_url too long",
			field: "source_url",
			body: createAppRequest{
				Name:      "valid-app",
				SourceURL: strings.Repeat("x", 2049),
			},
			wantErr: "source_url",
		},
		{
			name:  "branch too long",
			field: "branch",
			body: createAppRequest{
				Name:   "valid-app",
				Branch: strings.Repeat("x", 101),
			},
			wantErr: "branch",
		},
		{
			name:  "type too long",
			field: "type",
			body: createAppRequest{
				Name: "valid-app",
				Type: strings.Repeat("x", 51),
			},
			wantErr: "type",
		},
		{
			name:  "source_type too long",
			field: "source_type",
			body: createAppRequest{
				Name:       "valid-app",
				SourceType: strings.Repeat("x", 51),
			},
			wantErr: "source_type",
		},
		{
			name:  "project_id too long",
			field: "project_id",
			body: createAppRequest{
				Name:      "valid-app",
				ProjectID: strings.Repeat("x", 101),
			},
			wantErr: "project_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(bodyBytes))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}

			var resp map[string]any
			json.Unmarshal(rr.Body.Bytes(), &resp)
			errObj, _ := resp["error"].(map[string]any)
			if errObj["code"] != "validation_error" {
				t.Errorf("expected code=validation_error, got %v", errObj["code"])
			}

			// Check the field name appears in the details
			details, ok := errObj["details"].([]any)
			if !ok || len(details) == 0 {
				t.Fatal("expected field error details")
			}
			first, _ := details[0].(map[string]any)
			if first["field"] != tt.wantErr {
				t.Errorf("expected field=%q, got %v", tt.wantErr, first["field"])
			}
		})
	}
}

func TestCreateApp_ValidFieldLengths_Passes(t *testing.T) {
	store := newMockStore()
	c := &core.Core{
		Config: &core.Config{},
		Events: core.NewEventBus(nil),
	}
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:       "my-app",
		SourceURL:  "https://github.com/example/repo",
		Branch:     "main",
		Type:       "service",
		SourceType: "git",
		ProjectID:  "proj-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Database Creation Field Validation ─────────────────────────────────────

func TestCreateDB_FieldLengthValidation(t *testing.T) {
	handler := NewDatabaseHandler(nil, nil, nil)

	tests := []struct {
		name    string
		body    createDBRequest
		wantErr string
	}{
		{
			name:    "name too long",
			body:    createDBRequest{Name: strings.Repeat("x", 101), Engine: "postgres"},
			wantErr: "name",
		},
		{
			name:    "engine too long",
			body:    createDBRequest{Name: "mydb", Engine: strings.Repeat("x", 51)},
			wantErr: "engine",
		},
		{
			name:    "version too long",
			body:    createDBRequest{Name: "mydb", Engine: "postgres", Version: strings.Repeat("x", 51)},
			wantErr: "version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", bytes.NewReader(bodyBytes))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}

			var resp map[string]any
			json.Unmarshal(rr.Body.Bytes(), &resp)
			errObj, _ := resp["error"].(map[string]any)
			if errObj["code"] != "validation_error" {
				t.Errorf("expected code=validation_error, got %v", errObj["code"])
			}

			details, ok := errObj["details"].([]any)
			if !ok || len(details) == 0 {
				t.Fatal("expected field error details")
			}
			first, _ := details[0].(map[string]any)
			if first["field"] != tt.wantErr {
				t.Errorf("expected field=%q, got %v", tt.wantErr, first["field"])
			}
		})
	}
}

// ─── Server Provision Field Validation ──────────────────────────────────────

func TestProvision_FieldLengthValidation(t *testing.T) {
	services := core.NewServices()
	handler := NewServerHandler(nil, services, nil)

	tests := []struct {
		name    string
		body    provisionRequest
		wantErr string
	}{
		{
			name:    "name too long",
			body:    provisionRequest{Provider: "hetzner", Name: strings.Repeat("x", 101), Region: "fsn1", Size: "cx11"},
			wantErr: "name",
		},
		{
			name:    "provider too long",
			body:    provisionRequest{Provider: strings.Repeat("x", 51), Name: "srv", Region: "fsn1", Size: "cx11"},
			wantErr: "provider",
		},
		{
			name:    "region too long",
			body:    provisionRequest{Provider: "hetzner", Name: "srv", Region: strings.Repeat("x", 51), Size: "cx11"},
			wantErr: "region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(bodyBytes))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Provision(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}

			var resp map[string]any
			json.Unmarshal(rr.Body.Bytes(), &resp)
			errObj, _ := resp["error"].(map[string]any)
			if errObj["code"] != "validation_error" {
				t.Errorf("expected code=validation_error, got %v", errObj["code"])
			}
		})
	}
}

// ─── App Update Field Validation ────────────────────────────────────────────

func TestUpdateApp_FieldLengthValidation(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID: "app-1", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	c := &core.Core{
		Config: &core.Config{},
		Events: core.NewEventBus(nil),
	}
	handler := NewAppHandler(store, c)

	tests := []struct {
		name    string
		body    updateAppRequest
		wantErr string
	}{
		{
			name:    "source_url too long",
			body:    updateAppRequest{SourceURL: strings.Repeat("x", 2049)},
			wantErr: "source_url",
		},
		{
			name:    "branch too long",
			body:    updateAppRequest{Branch: strings.Repeat("x", 101)},
			wantErr: "branch",
		},
		{
			name:    "dockerfile too long",
			body:    updateAppRequest{Dockerfile: strings.Repeat("x", 501)},
			wantErr: "dockerfile",
		},
		{
			name:    "invalid name",
			body:    updateAppRequest{Name: "-invalid"},
			wantErr: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app-1", bytes.NewReader(bodyBytes))
			req.SetPathValue("id", "app-1")
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Update(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}

			var resp map[string]any
			json.Unmarshal(rr.Body.Bytes(), &resp)
			errObj, _ := resp["error"].(map[string]any)
			if errObj["code"] != "validation_error" {
				t.Errorf("expected code=validation_error, got %v", errObj["code"])
			}

			details, ok := errObj["details"].([]any)
			if !ok || len(details) == 0 {
				t.Fatal("expected field error details")
			}
			first, _ := details[0].(map[string]any)
			if first["field"] != tt.wantErr {
				t.Errorf("expected field=%q, got %v", tt.wantErr, first["field"])
			}
		})
	}
}

func TestUpdateApp_ReplicasOutOfRange(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{
		ID: "app-1", TenantID: "tenant1", Name: "my-app", Status: "running",
	}
	c := &core.Core{
		Config: &core.Config{},
		Events: core.NewEventBus(nil),
	}
	handler := NewAppHandler(store, c)

	badReplicas := 200
	body, _ := json.Marshal(updateAppRequest{Replicas: &badReplicas})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app-1", bytes.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
