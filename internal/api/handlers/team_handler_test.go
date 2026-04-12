package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── ListRoles ───────────────────────────────────────────────────────────────

func TestListRoles_Success(t *testing.T) {
	store := newMockStore()
	store.addRole("tenant1", core.Role{ID: "role_owner", TenantID: "tenant1", Name: "Owner", IsBuiltin: true})
	store.addRole("tenant1", core.Role{ID: "role_admin", TenantID: "tenant1", Name: "Admin", IsBuiltin: true})
	store.addRole("tenant1", core.Role{ID: "role_dev", TenantID: "tenant1", Name: "Developer", IsBuiltin: false})

	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/roles", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ListRoles(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 3 {
		t.Errorf("expected 3 roles, got %d", len(data))
	}

	total, ok := resp["total"].(float64)
	if !ok {
		t.Fatal("expected total in response")
	}
	if int(total) != 3 {
		t.Errorf("expected total 3, got %d", int(total))
	}
}

func TestListRoles_Empty(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/roles", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ListRoles(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total, _ := resp["total"].(float64)
	if int(total) != 0 {
		t.Errorf("expected total 0, got %d", int(total))
	}
}

func TestListRoles_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/roles", nil)
	rr := httptest.NewRecorder()

	handler.ListRoles(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestListRoles_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListRoles = errors.New("db connection lost")

	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/roles", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.ListRoles(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "internal error")
}

// ─── GetAuditLog ─────────────────────────────────────────────────────────────

func TestGetAuditLog_Success(t *testing.T) {
	store := newMockStore()
	store.addAuditLog("tenant1", core.AuditEntry{
		ID: 1, TenantID: "tenant1", UserID: "user1",
		Action: "app.create", ResourceType: "app", ResourceID: "app1",
	})
	store.addAuditLog("tenant1", core.AuditEntry{
		ID: 2, TenantID: "tenant1", UserID: "user1",
		Action: "app.deploy", ResourceType: "app", ResourceID: "app1",
	})

	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/audit-log", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetAuditLog(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 audit entries, got %d", len(data))
	}

	total, _ := resp["total"].(float64)
	if int(total) != 2 {
		t.Errorf("expected total 2, got %d", int(total))
	}
}

func TestGetAuditLog_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/audit-log", nil)
	rr := httptest.NewRecorder()

	handler.GetAuditLog(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestGetAuditLog_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListAuditLogs = errors.New("db error")

	events := core.NewEventBus(slog.Default())
	handler := NewTeamHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/audit-log", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetAuditLog(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "internal error")
}
