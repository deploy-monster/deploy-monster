package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ── Project field validation ────────────────────────────────────────────────

func TestProjectCreate_NameTooLong(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewProjectHandler(store, events)

	longName := strings.Repeat("a", 101)
	body, _ := json.Marshal(map[string]string{"name": longName})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "name must be 100 characters or less")
}

func TestProjectCreate_DescriptionTooLong(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewProjectHandler(store, events)

	longDesc := strings.Repeat("x", 501)
	body, _ := json.Marshal(map[string]string{"name": "valid", "description": longDesc})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "description must be 500 characters or less")
}

func TestProjectCreate_EmitsEvent(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	var received []core.Event
	events.Subscribe(core.EventProjectCreated, func(_ context.Context, evt core.Event) error {
		received = append(received, evt)
		return nil
	})

	h := NewProjectHandler(store, events)

	body, _ := json.Marshal(map[string]string{"name": "my-project"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != core.EventProjectCreated {
		t.Errorf("event type = %q, want %q", received[0].Type, core.EventProjectCreated)
	}
}

// ── Redirect field validation ───────────────────────────────────────────────

func TestRedirectCreate_SourceTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test"})
	h := NewRedirectHandler(store, newMockBoltStore())

	longSource := "/" + strings.Repeat("a", 2048)
	body, _ := json.Marshal(map[string]string{"source": longSource, "destination": "/new"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/redirects", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "source must be 2048 characters or less")
}

// ── BasicAuth field validation ──────────────────────────────────────────────

func TestBasicAuthUpdate_RealmTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test"})
	h := NewBasicAuthHandler(store, newMockBoltStore())

	longRealm := strings.Repeat("r", 101)
	body, _ := json.Marshal(map[string]any{
		"enabled": true,
		"realm":   longRealm,
		"users":   map[string]string{"admin": "$2b$10$hash"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/basic-auth", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "realm must be 100 characters or less")
}

func TestBasicAuthUpdate_TooManyUsers(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test"})
	h := NewBasicAuthHandler(store, newMockBoltStore())

	users := make(map[string]string)
	for i := 0; i < 51; i++ {
		users[strings.Repeat("u", 5)+string(rune('A'+i%26))+string(rune('0'+i/26))] = "$2b$hash"
	}

	body, _ := json.Marshal(map[string]any{
		"enabled": true,
		"users":   users,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/basic-auth", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "maximum 50 users allowed")
}
