package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Search ──────────────────────────────────────────────────────────────────

func TestSearch_MatchesApps(t *testing.T) {
	store := newMockStore()
	store.appList = []core.Application{
		{ID: "app1", TenantID: "tenant1", Name: "My Web App", Status: "running"},
		{ID: "app2", TenantID: "tenant1", Name: "API Service", Status: "running"},
		{ID: "app3", TenantID: "tenant1", Name: "Another Web Service", Status: "stopped"},
	}
	store.appTotal = 3

	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=web", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	results, ok := resp["results"].([]any)
	if !ok {
		t.Fatal("expected results array in response")
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'web', got %d", len(results))
	}

	if resp["query"] != "web" {
		t.Errorf("expected query 'web', got %v", resp["query"])
	}
}

func TestSearch_MatchesDomains(t *testing.T) {
	store := newMockStore()
	store.addDomain(&core.Domain{ID: "d1", FQDN: "example.com", Type: "primary", AppID: "app1"})
	store.addDomain(&core.Domain{ID: "d2", FQDN: "api.example.com", Type: "alias", AppID: "app1"})
	store.addDomain(&core.Domain{ID: "d3", FQDN: "other.org", Type: "primary", AppID: "app2"})

	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=example", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	results := resp["results"].([]any)
	if len(results) != 2 {
		t.Errorf("expected 2 domain results for 'example', got %d", len(results))
	}
}

func TestSearch_MatchesProjects(t *testing.T) {
	store := newMockStore()
	store.addProject("tenant1", core.Project{ID: "p1", TenantID: "tenant1", Name: "Production", Environment: "prod"})
	store.addProject("tenant1", core.Project{ID: "p2", TenantID: "tenant1", Name: "Staging", Environment: "staging"})

	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=prod", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	results := resp["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 project result for 'prod', got %d", len(results))
	}
}

func TestSearch_MixedResults(t *testing.T) {
	store := newMockStore()
	store.appList = []core.Application{
		{ID: "app1", TenantID: "tenant1", Name: "Test App", Status: "running"},
	}
	store.appTotal = 1
	store.addDomain(&core.Domain{ID: "d1", FQDN: "test.example.com", Type: "primary", AppID: "app1"})
	store.addProject("tenant1", core.Project{ID: "p1", TenantID: "tenant1", Name: "Test Project", Environment: "dev"})

	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	results := resp["results"].([]any)
	if len(results) != 3 {
		t.Errorf("expected 3 results (1 app + 1 domain + 1 project), got %d", len(results))
	}

	total, _ := resp["total"].(float64)
	if int(total) != 3 {
		t.Errorf("expected total 3, got %d", int(total))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	store := newMockStore()
	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "query must be at least 2 characters")
}

func TestSearch_QueryTooShort(t *testing.T) {
	store := newMockStore()
	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=a", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "query must be at least 2 characters")
}

func TestSearch_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestSearch_NoResults(t *testing.T) {
	store := newMockStore()
	handler := NewSearchHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=zzzzzzzzz", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

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
