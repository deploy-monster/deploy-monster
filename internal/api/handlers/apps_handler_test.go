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

// ─── List Apps ───────────────────────────────────────────────────────────────

func TestListApps_Success(t *testing.T) {
	store := newMockStore()
	store.appList = []core.Application{
		{ID: "app1", Name: "App One", TenantID: "tenant1", Status: "running"},
		{ID: "app2", Name: "App Two", TenantID: "tenant1", Status: "stopped"},
	}
	store.appTotal = 2

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
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

	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 apps in data, got %v", resp["data"])
	}
}

func TestListApps_Pagination(t *testing.T) {
	store := newMockStore()
	// Seed 5 apps
	for i := range 5 {
		store.appList = append(store.appList, core.Application{
			ID: core.GenerateID(), Name: "App", TenantID: "tenant1",
			Status: "running", Replicas: i + 1,
		})
	}
	store.appTotal = 5

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps?page=2&per_page=2", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	page := int(resp["page"].(float64))
	if page != 2 {
		t.Errorf("expected page=2, got %d", page)
	}

	perPage := int(resp["per_page"].(float64))
	if perPage != 2 {
		t.Errorf("expected per_page=2, got %d", perPage)
	}

	totalPages := int(resp["total_pages"].(float64))
	if totalPages != 3 {
		t.Errorf("expected total_pages=3, got %d", totalPages)
	}

	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 apps on page 2, got %d", len(data))
	}
}

func TestListApps_DefaultPagination(t *testing.T) {
	store := newMockStore()
	store.appTotal = 0

	c := testCore()
	handler := NewAppHandler(store, c)

	// No page or per_page params — defaults to page=1, per_page=20
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if int(resp["page"].(float64)) != 1 {
		t.Errorf("expected default page=1")
	}
	if int(resp["per_page"].(float64)) != 20 {
		t.Errorf("expected default per_page=20")
	}
}

func TestListApps_NoClaims(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestListApps_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListAppsByTenant = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Create App ──────────────────────────────────────────────────────────────

func TestCreateApp_Success(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{
		Name:       "my-web-app",
		Type:       "service",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo",
		Branch:     "main",
		ProjectID:  "proj1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var app core.Application
	json.Unmarshal(rr.Body.Bytes(), &app)

	if app.Name != "my-web-app" {
		t.Errorf("expected name my-web-app, got %q", app.Name)
	}
	if app.TenantID != "tenant1" {
		t.Errorf("expected tenant_id tenant1, got %q", app.TenantID)
	}
	if app.Status != "pending" {
		t.Errorf("expected status pending, got %q", app.Status)
	}
	if app.Replicas != 1 {
		t.Errorf("expected replicas 1, got %d", app.Replicas)
	}
	if app.ID == "" {
		t.Error("expected non-empty app ID")
	}

	// Verify the app was stored.
	if store.createdApp == nil {
		t.Fatal("expected app to be stored")
	}
}

func TestCreateApp_Defaults(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	// Only name provided — type, source_type, branch should get defaults.
	body, _ := json.Marshal(createAppRequest{Name: "minimal-app"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var app core.Application
	json.Unmarshal(rr.Body.Bytes(), &app)

	if app.Type != "service" {
		t.Errorf("expected default type 'service', got %q", app.Type)
	}
	if app.SourceType != "image" {
		t.Errorf("expected default source_type 'image', got %q", app.SourceType)
	}
	if app.Branch != "main" {
		t.Errorf("expected default branch 'main', got %q", app.Branch)
	}
}

func TestCreateApp_MissingName(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{Type: "service"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "name is required")
}

func TestCreateApp_InvalidJSON(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader([]byte("{")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCreateApp_NoClaims(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{Name: "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCreateApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.errCreateApp = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	body, _ := json.Marshal(createAppRequest{Name: "fail-app"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Get App ─────────────────────────────────────────────────────────────────

func TestGetApp_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		Name:     "Test App",
		TenantID: "tenant1",
		Status:   "running",
		Type:     "service",
	})

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var app core.Application
	json.Unmarshal(rr.Body.Bytes(), &app)

	if app.ID != "app1" {
		t.Errorf("expected id app1, got %q", app.ID)
	}
	if app.Name != "Test App" {
		t.Errorf("expected name 'Test App', got %q", app.Name)
	}
}

func TestGetApp_NotFound(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "application not found")
}

func TestGetApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.errGetApp = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Delete App ──────────────────────────────────────────────────────────────

func TestDeleteApp_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Doomed"})

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.deletedAppID != "app1" {
		t.Errorf("expected deleted app ID app1, got %q", store.deletedAppID)
	}
}

func TestDeleteApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test"})
	store.errDeleteApp = errors.New("constraint violation")

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Restart / Stop / Start ──────────────────────────────────────────────────

func TestRestart_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restart(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if store.updatedStatus["app1"] != "running" {
		t.Errorf("expected status 'running', got %q", store.updatedStatus["app1"])
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "restarting" {
		t.Errorf("expected response status 'restarting', got %q", resp["status"])
	}
}

func TestRestart_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	store.errUpdateAppStatus = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestStop_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stop(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if store.updatedStatus["app1"] != "stopped" {
		t.Errorf("expected status 'stopped', got %q", store.updatedStatus["app1"])
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "stopped" {
		t.Errorf("expected response status 'stopped', got %q", resp["status"])
	}
}

func TestStop_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "running"})
	store.errUpdateAppStatus = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Stop(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestStart_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "stopped"})
	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if store.updatedStatus["app1"] != "running" {
		t.Errorf("expected status 'running', got %q", store.updatedStatus["app1"])
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "running" {
		t.Errorf("expected response status 'running', got %q", resp["status"])
	}
}

func TestStart_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1", Name: "Test", Status: "stopped"})
	store.errUpdateAppStatus = errors.New("db error")

	c := testCore()
	handler := NewAppHandler(store, c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Start(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Integration: Create then Get ────────────────────────────────────────────

func TestCreateThenGet_Integration(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	// Create
	body, _ := json.Marshal(createAppRequest{Name: "integrated-app", Type: "worker"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	createReq = withClaims(createReq, "user1", "tenant1", "role_owner", "user@example.com")
	createRR := httptest.NewRecorder()

	handler.Create(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created core.Application
	json.Unmarshal(createRR.Body.Bytes(), &created)

	// Get
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/apps/"+created.ID, nil)
	getReq.SetPathValue("id", created.ID)
	getReq = withClaims(getReq, "user1", "tenant1", "role_owner", "user@example.com")
	getRR := httptest.NewRecorder()

	handler.Get(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", getRR.Code)
	}

	var fetched core.Application
	json.Unmarshal(getRR.Body.Bytes(), &fetched)

	if fetched.ID != created.ID {
		t.Errorf("expected same ID, got %q vs %q", fetched.ID, created.ID)
	}
	if fetched.Name != "integrated-app" {
		t.Errorf("expected name 'integrated-app', got %q", fetched.Name)
	}
	if fetched.Type != "worker" {
		t.Errorf("expected type 'worker', got %q", fetched.Type)
	}
}

// ─── Integration: Create then Delete then Get (404) ──────────────────────────

func TestCreateDeleteGet_Integration(t *testing.T) {
	store := newMockStore()
	c := testCore()
	handler := NewAppHandler(store, c)

	// Create
	body, _ := json.Marshal(createAppRequest{Name: "ephemeral"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/apps", bytes.NewReader(body))
	createReq = withClaims(createReq, "user1", "tenant1", "role_owner", "user@example.com")
	createRR := httptest.NewRecorder()
	handler.Create(createRR, createReq)

	var created core.Application
	json.Unmarshal(createRR.Body.Bytes(), &created)

	// Delete
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/"+created.ID, nil)
	delReq.SetPathValue("id", created.ID)
	delReq = withClaims(delReq, "user1", "tenant1", "role_owner", "user@example.com")
	delRR := httptest.NewRecorder()
	handler.Delete(delRR, delReq)

	if delRR.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", delRR.Code)
	}

	// Get should now 404
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/apps/"+created.ID, nil)
	getReq.SetPathValue("id", created.ID)
	getReq = withClaims(getReq, "user1", "tenant1", "role_owner", "user@example.com")
	getRR := httptest.NewRecorder()
	handler.Get(getRR, getReq)

	if getRR.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", getRR.Code)
	}
}
