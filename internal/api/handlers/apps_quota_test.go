package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestCreateApp_QuotaEnforced_AtLimit is the Phase 3.3.6 headline for
// the app-count half of quota enforcement: a tenant sitting at its
// Config.Limits.MaxAppsPerTenant ceiling must not be able to create
// one more. The handler is expected to return 429 Too Many Requests
// and must NOT call CreateApp on the store — otherwise a racing pair
// of N+1 requests could both slip past the check.
func TestCreateApp_QuotaEnforced_AtLimit(t *testing.T) {
	store := newMockStore()
	// Seed two existing apps under the same tenant so appTotal = 2.
	store.appList = []core.Application{
		{ID: "app1", Name: "existing-1", TenantID: "tenant1", Status: "running"},
		{ID: "app2", Name: "existing-2", TenantID: "tenant1", Status: "running"},
	}
	store.appTotal = 2

	c := testCore()
	c.Config.Limits.MaxAppsPerTenant = 2 // at limit
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:       "one-too-many",
		Type:       "service",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo",
		Branch:     "main",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "quota") {
		t.Errorf("error body should mention quota, got %s", rr.Body.String())
	}
	if store.createdApp != nil {
		t.Errorf("CreateApp was called with %+v — quota should have short-circuited", store.createdApp)
	}
}

// TestCreateApp_QuotaEnforced_UnderLimit proves the gate is opt-in
// per-dimension: a tenant under limit flows through to the normal
// CreateApp path and gets a 201.
func TestCreateApp_QuotaEnforced_UnderLimit(t *testing.T) {
	store := newMockStore()
	store.appList = []core.Application{
		{ID: "app1", Name: "first", TenantID: "tenant1", Status: "running"},
	}
	store.appTotal = 1

	c := testCore()
	c.Config.Limits.MaxAppsPerTenant = 5 // plenty of room
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:       "second",
		Type:       "service",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.createdApp == nil {
		t.Fatal("CreateApp was not called")
	}
	if store.createdApp.Name != "second" {
		t.Errorf("created app name = %q, want second", store.createdApp.Name)
	}
}

// TestCreateApp_QuotaEnforced_ZeroLimitIsUnlimited guards the
// backwards-compat invariant: a fresh config that never set
// MaxAppsPerTenant (zero value) must not suddenly refuse every
// create. Zero means "unlimited" to match the billing.Plan
// convention for enterprise plans.
func TestCreateApp_QuotaEnforced_ZeroLimitIsUnlimited(t *testing.T) {
	store := newMockStore()
	// Seed a ridiculous number of existing apps.
	for i := 0; i < 1000; i++ {
		store.appList = append(store.appList, core.Application{
			ID: core.GenerateID(), Name: "existing", TenantID: "tenant1",
		})
	}
	store.appTotal = 1000

	c := testCore()
	c.Config.Limits.MaxAppsPerTenant = 0 // unlimited
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{Name: "yet-another", Type: "service"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("zero limit should allow unlimited creates, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestCreateApp_QuotaEnforced_OverLimit verifies the gate does NOT
// rely on exact equality — a tenant whose current count already
// exceeds the limit (e.g. because the limit was lowered mid-flight)
// must still be blocked on every new create.
func TestCreateApp_QuotaEnforced_OverLimit(t *testing.T) {
	store := newMockStore()
	store.appList = []core.Application{
		{ID: "app1", Name: "a", TenantID: "tenant1"},
		{ID: "app2", Name: "b", TenantID: "tenant1"},
		{ID: "app3", Name: "c", TenantID: "tenant1"},
	}
	store.appTotal = 3

	c := testCore()
	c.Config.Limits.MaxAppsPerTenant = 2 // already over
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{Name: "d", Type: "service"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for over-limit tenant, got %d: %s", rr.Code, rr.Body.String())
	}
}
