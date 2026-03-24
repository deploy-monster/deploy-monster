package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/billing"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── ListPlans ───────────────────────────────────────────────────────────────

func TestListPlans_ReturnsAllPlans(t *testing.T) {
	store := newMockStore()
	handler := NewBillingHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/plans", nil)
	rr := httptest.NewRecorder()

	handler.ListPlans(rr, req)

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

	if len(data) != len(billing.BuiltinPlans) {
		t.Errorf("expected %d plans, got %d", len(billing.BuiltinPlans), len(data))
	}
}

// ─── GetUsage ────────────────────────────────────────────────────────────────

func TestGetUsage_Success(t *testing.T) {
	store := newMockStore()
	store.addTenant(&core.Tenant{ID: "tenant1", PlanID: "free"})

	// Seed some apps so usage is non-zero.
	store.appList = []core.Application{
		{ID: "app1", TenantID: "tenant1", Name: "App One"},
		{ID: "app2", TenantID: "tenant1", Name: "App Two"},
	}
	store.appTotal = 2

	handler := NewBillingHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetUsage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["apps_used"] == nil {
		t.Error("expected apps_used in response")
	}
	if resp["plan"] == nil {
		t.Error("expected plan in response")
	}
	if resp["quota"] == nil {
		t.Error("expected quota in response")
	}

	// Verify apps_used is correct.
	appsUsed, ok := resp["apps_used"].(float64)
	if !ok {
		t.Fatalf("expected apps_used to be a number, got %T", resp["apps_used"])
	}
	if int(appsUsed) != 2 {
		t.Errorf("expected apps_used 2, got %d", int(appsUsed))
	}
}

func TestGetUsage_DefaultsToFreePlan(t *testing.T) {
	store := newMockStore()
	// Tenant with unknown plan ID should default to free.
	store.addTenant(&core.Tenant{ID: "tenant1", PlanID: "nonexistent-plan"})

	handler := NewBillingHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetUsage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	planMap, ok := resp["plan"].(map[string]any)
	if !ok {
		t.Fatal("expected plan object in response")
	}
	if planMap["id"] != "free" {
		t.Errorf("expected default plan to be 'free', got %v", planMap["id"])
	}
}

func TestGetUsage_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewBillingHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	rr := httptest.NewRecorder()

	handler.GetUsage(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestGetUsage_TenantNotFound(t *testing.T) {
	store := newMockStore()
	// No tenant seeded — store returns ErrNotFound.
	handler := NewBillingHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	req = withClaims(req, "user1", "tenant-missing", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.GetUsage(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "internal error")
}
