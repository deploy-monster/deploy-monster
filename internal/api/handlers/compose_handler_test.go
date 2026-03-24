package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// validComposeYAML is a minimal valid docker-compose YAML for tests.
const validComposeYAML = `services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
  api:
    image: node:20
    depends_on:
      - web
`

const invalidComposeYAML = `not: valid: yaml: [[[`

// ─── Deploy ──────────────────────────────────────────────────────────────────

func TestComposeDeploy_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	// runtime is nil — Deploy fires a goroutine that uses runtime,
	// but the handler returns 202 Accepted immediately so it doesn't block.
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{
		"name": "my-stack",
		"yaml": validComposeYAML,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["name"] != "my-stack" {
		t.Errorf("expected name my-stack, got %v", resp["name"])
	}
	if resp["status"] != "deploying" {
		t.Errorf("expected status deploying, got %v", resp["status"])
	}
	services := int(resp["services"].(float64))
	if services != 2 {
		t.Errorf("expected 2 services, got %d", services)
	}
	if resp["app_id"] == nil || resp["app_id"] == "" {
		t.Error("expected non-empty app_id")
	}
}

func TestComposeDeploy_AutoGeneratesName(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{
		"yaml": validComposeYAML,
		// name intentionally omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	name, ok := resp["name"].(string)
	if !ok || name == "" {
		t.Error("expected auto-generated name")
	}
	if len(name) < 10 {
		t.Errorf("expected auto-generated name with prefix 'stack-', got %q", name)
	}
}

func TestComposeDeploy_NoClaims(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{"yaml": validComposeYAML})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestComposeDeploy_InvalidJSON(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", bytes.NewReader([]byte("bad json")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestComposeDeploy_EmptyYAML(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{"name": "stack", "yaml": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "yaml is required")
}

func TestComposeDeploy_InvalidYAML(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{
		"name": "bad-stack",
		"yaml": invalidComposeYAML,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Validate ────────────────────────────────────────────────────────────────

func TestComposeValidate_ValidYAML(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{"yaml": validComposeYAML})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/validate", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Validate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v", resp["valid"])
	}
	services := int(resp["services"].(float64))
	if services != 2 {
		t.Errorf("expected 2 services, got %d", services)
	}
}

func TestComposeValidate_InvalidYAML(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	body, _ := json.Marshal(map[string]string{"yaml": invalidComposeYAML})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/validate", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Validate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["valid"] != false {
		t.Errorf("expected valid=false, got %v", resp["valid"])
	}
	if resp["error"] == nil || resp["error"] == "" {
		t.Error("expected error message for invalid YAML")
	}
}

func TestComposeValidate_InvalidJSON(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/validate", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()

	handler.Validate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// ─── YAML Content-Type ───────────────────────────────────────────────────────

func TestComposeDeploy_YAMLContentType(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewComposeHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks?name=yaml-stack", bytes.NewReader([]byte(validComposeYAML)))
	req.Header.Set("Content-Type", "application/x-yaml")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Deploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["name"] != "yaml-stack" {
		t.Errorf("expected name yaml-stack, got %v", resp["name"])
	}
}
