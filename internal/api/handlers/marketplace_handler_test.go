package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// newTestRegistry creates a TemplateRegistry pre-seeded with test templates.
func newTestRegistry() *marketplace.TemplateRegistry {
	reg := marketplace.NewTemplateRegistry()
	reg.Add(&marketplace.Template{
		Slug: "postgres", Name: "PostgreSQL", Description: "Reliable relational database",
		Category: "database", Tags: []string{"sql", "relational"}, Version: "16",
		Author: "community", Verified: true,
	})
	reg.Add(&marketplace.Template{
		Slug: "redis", Name: "Redis", Description: "In-memory key-value store",
		Category: "database", Tags: []string{"cache", "nosql"}, Version: "7",
		Author: "community", Verified: true,
	})
	reg.Add(&marketplace.Template{
		Slug: "nginx", Name: "Nginx", Description: "High-performance web server",
		Category: "webserver", Tags: []string{"proxy", "http"}, Version: "1.27",
		Author: "community", Verified: true,
	})
	return reg
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestMarketplaceList_AllTemplates(t *testing.T) {
	reg := newTestRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

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
		t.Errorf("expected 3 templates, got %d", len(data))
	}

	total, _ := resp["total"].(float64)
	if int(total) != 3 {
		t.Errorf("expected total 3, got %d", int(total))
	}

	cats, ok := resp["categories"].([]any)
	if !ok {
		t.Fatal("expected categories array in response")
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 categories (database, webserver), got %d", len(cats))
	}
}

func TestMarketplaceList_FilterByCategory(t *testing.T) {
	reg := newTestRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace?category=database", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 database templates, got %d", len(data))
	}
}

func TestMarketplaceList_SearchQuery(t *testing.T) {
	reg := newTestRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace?q=redis", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 result for 'redis', got %d", len(data))
	}
}

func TestMarketplaceList_SearchNoResults(t *testing.T) {
	reg := newTestRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace?q=nonexistent", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

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

func TestMarketplaceList_EmptyRegistry(t *testing.T) {
	reg := marketplace.NewTemplateRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

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

// ─── Get ─────────────────────────────────────────────────────────────────────

func TestMarketplaceGet_Found(t *testing.T) {
	reg := newTestRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/postgres", nil)
	req.SetPathValue("slug", "postgres")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var tmpl marketplace.Template
	if err := json.Unmarshal(rr.Body.Bytes(), &tmpl); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if tmpl.Slug != "postgres" {
		t.Errorf("expected slug 'postgres', got %q", tmpl.Slug)
	}
	if tmpl.Name != "PostgreSQL" {
		t.Errorf("expected name 'PostgreSQL', got %q", tmpl.Name)
	}
}

func TestMarketplaceGet_NotFound(t *testing.T) {
	reg := newTestRegistry()
	handler := NewMarketplaceHandler(reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/marketplace/nonexistent", nil)
	req.SetPathValue("slug", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "template not found")
}
