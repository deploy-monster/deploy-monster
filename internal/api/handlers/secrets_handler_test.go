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

// ─── Mock Vault ──────────────────────────────────────────────────────────────

type testVault struct {
	encryptErr error
	decryptErr error
}

func (v *testVault) Encrypt(plaintext string) (string, error) {
	if v.encryptErr != nil {
		return "", v.encryptErr
	}
	return "enc:" + plaintext, nil
}

func (v *testVault) Decrypt(ciphertext string) (string, error) {
	if v.decryptErr != nil {
		return "", v.decryptErr
	}
	if len(ciphertext) > 4 {
		return ciphertext[4:], nil
	}
	return ciphertext, nil
}

// ─── Create Secret ───────────────────────────────────────────────────────────

func TestCreateSecret_Success(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{
		Name:        "DB_PASSWORD",
		Value:       "super-secret-123",
		Description: "Database password",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["name"] != "DB_PASSWORD" {
		t.Errorf("expected name DB_PASSWORD, got %v", resp["name"])
	}
	if resp["scope"] != "tenant" {
		t.Errorf("expected default scope 'tenant', got %v", resp["scope"])
	}
	if resp["reference"] != "${SECRET:DB_PASSWORD}" {
		t.Errorf("expected reference ${SECRET:DB_PASSWORD}, got %v", resp["reference"])
	}
	if resp["encrypted"] != true {
		t.Errorf("expected encrypted=true, got %v", resp["encrypted"])
	}
	if resp["description"] != "Database password" {
		t.Errorf("expected description 'Database password', got %v", resp["description"])
	}
}

func TestCreateSecret_WithScope(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{
		Name:  "API_KEY",
		Value: "key-456",
		Scope: "global",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["scope"] != "global" {
		t.Errorf("expected scope global, got %v", resp["scope"])
	}
}

func TestCreateSecret_AppScopeRequiresTenantApp(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "foreign-app", TenantID: "tenant2"})
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{
		Name:  "APP_SECRET",
		Value: "value",
		Scope: "app",
		AppID: "foreign-app",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateSecret_ProjectScopeRequiresTenantProject(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "foreign-project", TenantID: "tenant2", Name: "Foreign"})
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{
		Name:      "PROJECT_SECRET",
		Value:     "value",
		Scope:     "project",
		ProjectID: "foreign-project",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateSecret_AppScopeRequiresAppID(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{Name: "APP_SECRET", Value: "value", Scope: "app"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "app_id is required for app-scoped secrets")
}

func TestCreateSecret_NilVault(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	// vault is nil — value should be stored as-is (no encryption)
	handler := NewSecretHandler(store, nil, events)

	body, _ := json.Marshal(createSecretRequest{
		Name:  "PLAIN_SECRET",
		Value: "not-encrypted",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateSecret_NoClaims(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{Name: "X", Value: "Y"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCreateSecret_InvalidJSON(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader([]byte("bad")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCreateSecret_MissingFields(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	tests := []struct {
		name string
		body createSecretRequest
	}{
		{"missing name", createSecretRequest{Value: "val"}},
		{"missing value", createSecretRequest{Name: "key"}},
		{"both empty", createSecretRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
			assertErrorMessage(t, rr, "name and value are required")
		})
	}
}

func TestCreateSecret_EncryptionError(t *testing.T) {
	store := newMockStore()
	vault := &testVault{encryptErr: errors.New("HSM unavailable")}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	body, _ := json.Marshal(createSecretRequest{Name: "KEY", Value: "val"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "encryption failed")
}

// ─── List Secrets ────────────────────────────────────────────────────────────

func TestListSecrets_Success(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["total"] == nil {
		t.Error("expected total field in response")
	}
	if resp["data"] == nil {
		t.Error("expected data field in response")
	}
}

func TestListSecrets_NoClaims(t *testing.T) {
	store := newMockStore()
	vault := &testVault{}
	events := core.NewEventBus(nil)
	handler := NewSecretHandler(store, vault, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
