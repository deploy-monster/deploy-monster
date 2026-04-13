package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Pagination Safety Tests ───────────────────────────────────────────────

func TestProjectsList_Paginated(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewProjectHandler(store, events)

	// Seed 25 projects
	for i := 0; i < 25; i++ {
		store.projects["tenant1"] = append(store.projects["tenant1"], core.Project{
			ID: "p-" + string(rune('A'+i)), TenantID: "tenant1", Name: "proj",
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects?per_page=10&page=1", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, _ := resp["data"].([]any)
	if len(data) != 10 {
		t.Errorf("expected 10 items, got %d", len(data))
	}
	if total, _ := resp["total"].(float64); total != 25 {
		t.Errorf("expected total=25, got %v", total)
	}
	if pages, _ := resp["total_pages"].(float64); pages != 3 {
		t.Errorf("expected total_pages=3, got %v", pages)
	}
}

func TestProjectsList_DefaultPagination(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewProjectHandler(store, events)

	// Seed 50 projects
	for i := 0; i < 50; i++ {
		store.projects["tenant1"] = append(store.projects["tenant1"], core.Project{
			ID: "p", TenantID: "tenant1",
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, _ := resp["data"].([]any)
	if len(data) != 20 {
		t.Errorf("expected default 20 items, got %d", len(data))
	}
}

func TestDomainsList_Paginated(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	// Seed an app for tenant1 so domain filtering works
	store.apps["app1"] = &core.Application{ID: "app1", TenantID: "tenant1"}

	// Seed 15 domains
	for i := 0; i < 15; i++ {
		d := &core.Domain{ID: "d-" + string(rune('A'+i)), AppID: "app1", FQDN: "test.com"}
		store.addDomain(d)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains?per_page=5&page=2", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, _ := resp["data"].([]any)
	if len(data) != 5 {
		t.Errorf("expected 5 items on page 2, got %d", len(data))
	}
	if total, _ := resp["total"].(float64); total != 15 {
		t.Errorf("expected total=15, got %v", total)
	}
}

// ─── Project Delete Audit Event Tests ──────────────────────────────────────

func TestProjectDelete_EmitsEvent(t *testing.T) {
	store := newMockStore()
	store.projectsByID["proj-1"] = &core.Project{ID: "proj-1", TenantID: "tenant1"}

	events := core.NewEventBus(nil)
	var received []core.Event
	events.Subscribe(core.EventProjectDeleted, func(_ context.Context, event core.Event) error {
		received = append(received, event)
		return nil
	})

	handler := NewProjectHandler(store, events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/proj-1", nil)
	req.SetPathValue("id", "proj-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != core.EventProjectDeleted {
		t.Errorf("expected event type %q, got %q", core.EventProjectDeleted, received[0].Type)
	}
}
