package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Dashboard Stats ─────────────────────────────────────────────────────────

func TestDashboardStats_Success(t *testing.T) {
	store := newMockStore()
	store.appList = []core.Application{
		{ID: "app1", Name: "App One", TenantID: "tenant1", Status: "running"},
		{ID: "app2", Name: "App Two", TenantID: "tenant1", Status: "stopped"},
		{ID: "app3", Name: "App Three", TenantID: "tenant1", Status: "running"},
	}
	store.appTotal = 3

	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "app1.example.com"})
	store.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "app2.example.com"})

	store.addProject("tenant1", core.Project{ID: "proj1", TenantID: "tenant1", Name: "Project One"})

	events := testCore().Events
	handler := NewDashboardHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	// Apps
	apps := resp["apps"].(map[string]any)
	if int(apps["total"].(float64)) != 3 {
		t.Errorf("expected apps.total=3, got %v", apps["total"])
	}

	// Domains
	domains := int(resp["domains"].(float64))
	if domains != 2 {
		t.Errorf("expected domains=2, got %d", domains)
	}

	// Projects
	projects := int(resp["projects"].(float64))
	if projects != 1 {
		t.Errorf("expected projects=1, got %d", projects)
	}

	// Containers (nil runtime, should be 0/0)
	containers := resp["containers"].(map[string]any)
	if int(containers["running"].(float64)) != 0 {
		t.Errorf("expected containers.running=0, got %v", containers["running"])
	}
	if int(containers["stopped"].(float64)) != 0 {
		t.Errorf("expected containers.stopped=0, got %v", containers["stopped"])
	}

	// Events
	ev := resp["events"].(map[string]any)
	if ev["published"] == nil {
		t.Error("expected events.published to be present")
	}
}

func TestDashboardStats_WithContainerRuntime(t *testing.T) {
	store := newMockStore()
	store.appTotal = 0

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.enable": "true"}},
			{ID: "c2", State: "running", Labels: map[string]string{"monster.enable": "true"}},
			{ID: "c3", State: "exited", Labels: map[string]string{"monster.enable": "true"}},
		},
	}

	events := testCore().Events
	handler := NewDashboardHandler(store, runtime, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	containers := resp["containers"].(map[string]any)
	if int(containers["running"].(float64)) != 2 {
		t.Errorf("expected containers.running=2, got %v", containers["running"])
	}
	if int(containers["stopped"].(float64)) != 1 {
		t.Errorf("expected containers.stopped=1, got %v", containers["stopped"])
	}
	if int(containers["total"].(float64)) != 3 {
		t.Errorf("expected containers.total=3, got %v", containers["total"])
	}
}

func TestDashboardStats_NoClaims(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDashboardHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	rr := httptest.NewRecorder()

	handler.Stats(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDashboardStats_EmptyData(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDashboardHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	apps := resp["apps"].(map[string]any)
	if int(apps["total"].(float64)) != 0 {
		t.Errorf("expected apps.total=0, got %v", apps["total"])
	}
	if int(resp["domains"].(float64)) != 0 {
		t.Errorf("expected domains=0, got %v", resp["domains"])
	}
	if int(resp["projects"].(float64)) != 0 {
		t.Errorf("expected projects=0, got %v", resp["projects"])
	}
}
