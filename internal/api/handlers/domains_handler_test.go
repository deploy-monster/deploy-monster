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

// ─── List Domains ────────────────────────────────────────────────────────────

func TestListDomains_All(t *testing.T) {
	store := newMockStore()
	// Seed apps so domain filtering by tenant works
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	store.addApp(&core.Application{ID: "app2", TenantID: "tenant1"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "example.com", Type: "custom"})
	store.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "api.example.com", Type: "custom"})

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total, ok := resp["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("expected total=2, got %v", resp["total"])
	}
}

func TestListDomains_ByApp(t *testing.T) {
	store := newMockStore()
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "example.com", Type: "custom"})
	store.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "other.com", Type: "custom"})

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains?app_id=app1", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 1 {
		t.Errorf("expected total=1 for app1, got %d", total)
	}
}

func TestListDomains_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestListDomains_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListAllDomains = errors.New("db error")

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestListDomains_ByApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListDomainsByApp = errors.New("db error")

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains?app_id=app1", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Create Domain ───────────────────────────────────────────────────────────

func TestCreateDomain_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	body, _ := json.Marshal(createDomainRequest{
		AppID: "app1",
		FQDN:  "myapp.example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var domain core.Domain
	json.Unmarshal(rr.Body.Bytes(), &domain)

	if domain.FQDN != "myapp.example.com" {
		t.Errorf("expected fqdn myapp.example.com, got %q", domain.FQDN)
	}
	if domain.AppID != "app1" {
		t.Errorf("expected app_id app1, got %q", domain.AppID)
	}
	if domain.Type != "custom" {
		t.Errorf("expected type custom, got %q", domain.Type)
	}
	if domain.DNSProvider != "manual" {
		t.Errorf("expected default dns_provider 'manual', got %q", domain.DNSProvider)
	}

	if store.createdDomain == nil {
		t.Fatal("expected domain to be stored")
	}
}

func TestCreateDomain_WithDNSProvider(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	body, _ := json.Marshal(createDomainRequest{
		AppID:       "app1",
		FQDN:        "myapp.example.com",
		DNSProvider: "cloudflare",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var domain core.Domain
	json.Unmarshal(rr.Body.Bytes(), &domain)

	if domain.DNSProvider != "cloudflare" {
		t.Errorf("expected dns_provider cloudflare, got %q", domain.DNSProvider)
	}
}

func TestCreateDomain_InvalidJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader([]byte("bad")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCreateDomain_MissingFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	tests := []struct {
		name string
		body createDomainRequest
	}{
		{"missing app_id", createDomainRequest{FQDN: "example.com"}},
		{"missing fqdn", createDomainRequest{AppID: "app1"}},
		{"both empty", createDomainRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
			assertErrorMessage(t, rr, "app_id and fqdn are required")
		})
	}
}

func TestCreateDomain_Duplicate(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "taken.com"})

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	body, _ := json.Marshal(createDomainRequest{AppID: "app1", FQDN: "taken.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "domain already exists")
}

func TestCreateDomain_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	store.errCreateDomain = errors.New("db error")

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	body, _ := json.Marshal(createDomainRequest{AppID: "app1", FQDN: "new.example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to create domain")
}

// ─── Delete Domain ───────────────────────────────────────────────────────────

func TestDeleteDomain_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.deletedDomainID != "d1" {
		t.Errorf("expected deleted domain ID d1, got %q", store.deletedDomainID)
	}
}

func TestDeleteDomain_NotFound(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "domain not found")
}

func TestDeleteDomain_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})
	store.errDeleteDomain = errors.New("constraint violation")

	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to delete domain")
}

// ─── Integration: Create then Delete ─────────────────────────────────────────

func TestCreateThenDeleteDomain_Integration(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "test-app"})
	events := core.NewEventBus(nil)
	handler := NewDomainHandler(store, events)

	// Create
	body, _ := json.Marshal(createDomainRequest{AppID: "app1", FQDN: "ephemeral.io"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	createReq = withClaims(createReq, "u1", "tenant1", "role_admin", "a@b.com")
	createRR := httptest.NewRecorder()
	handler.Create(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created core.Domain
	json.Unmarshal(createRR.Body.Bytes(), &created)

	// Delete
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+created.ID, nil)
	delReq.SetPathValue("id", created.ID)
	delReq = withClaims(delReq, "u1", "tenant1", "role_admin", "a@b.com")
	delRR := httptest.NewRecorder()
	handler.Delete(delRR, delReq)

	if delRR.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", delRR.Code)
	}

	// Verify domain is gone — attempting to delete again should 404.
	delReq2 := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+created.ID, nil)
	delReq2.SetPathValue("id", created.ID)
	delReq2 = withClaims(delReq2, "u1", "tenant1", "role_admin", "a@b.com")
	delRR2 := httptest.NewRecorder()
	handler.Delete(delRR2, delReq2)

	if delRR2.Code != http.StatusNotFound {
		t.Fatalf("second delete: expected 404, got %d", delRR2.Code)
	}
}
