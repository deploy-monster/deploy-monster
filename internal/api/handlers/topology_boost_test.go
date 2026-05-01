package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestTopologyHandler_Load_NotFound(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	// Seed a project for the tenant (so project check passes, but no topology exists)
	store.projectsByID["proj-1"] = &core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "Test Project"}

	req := httptest.NewRequest("GET", "/api/v1/topology/proj-1/production", nil)
	req.SetPathValue("projectId", "proj-1")
	req.SetPathValue("environment", "production")
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Load(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "No saved topology found" {
		t.Errorf("unexpected message: %v", resp["message"])
	}
}

func TestTopologyHandler_Load_Found(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	// Seed a project for the tenant
	store.projectsByID["proj-1"] = &core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "Test Project"}

	// Seed a saved topology
	key := "topology:tenant-1:proj-1:production"
	bolt.Set("topologies", key, TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}, 0)

	req := httptest.NewRequest("GET", "/api/v1/topology/proj-1/production", nil)
	req.SetPathValue("projectId", "proj-1")
	req.SetPathValue("environment", "production")
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Load(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "Topology loaded successfully" {
		t.Errorf("unexpected message: %v", resp["message"])
	}
}

func TestTopologyHandler_Load_NoClaims(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	req := httptest.NewRequest("GET", "/api/v1/topology/proj-1/production", nil)
	w := httptest.NewRecorder()
	h.Load(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Compile_Success(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{ID: "app-1", Type: "app", Position: Position{X: 100, Y: 100}, Data: map[string]any{
				"name":   "api",
				"gitUrl": "https://github.com/user/api",
				"branch": "main",
				"port":   3000,
			}},
		},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/compile", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Compile(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp CompileResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Success {
		t.Errorf("expected success, got: %s", resp.Message)
	}
}

func TestTopologyHandler_Compile_EmptyNodes(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{Nodes: []TopologyNode{}, ProjectID: "p1", Environment: "prod"}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/compile", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Compile(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Compile_NoClaims(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/compile", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.Compile(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Validate_Success(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{ID: "app-1", Type: "app", Position: Position{X: 100, Y: 100}, Data: map[string]any{
				"name":   "api",
				"gitUrl": "https://github.com/user/api",
				"branch": "main",
				"port":   3000,
			}},
		},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/validate", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Validate(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v", resp["valid"])
	}
}

func TestTopologyHandler_Validate_NoClaims(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/validate", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.Validate(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Templates(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	req := httptest.NewRequest("GET", "/api/v1/topology/templates", nil)
	w := httptest.NewRecorder()
	h.Templates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) == 0 {
		t.Error("expected templates to be returned")
	}
}

func TestTopologyHandler_convertToVolume(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	node := TopologyNode{
		ID:   "vol-1",
		Type: "volume",
		Data: map[string]any{
			"name":       "data-vol",
			"sizeGB":     20,
			"mountPath":  "/app/data",
			"volumeType": "nfs",
			"temporary":  true,
		},
	}

	vol := h.convertToVolume(node)
	if vol.ID != "vol-1" {
		t.Errorf("id = %q, want vol-1", vol.ID)
	}
	if vol.Name != "data-vol" {
		t.Errorf("name = %q, want data-vol", vol.Name)
	}
	if vol.SizeGB != 20 {
		t.Errorf("sizeGB = %d, want 20", vol.SizeGB)
	}
	if vol.MountPath != "/app/data" {
		t.Errorf("mountPath = %q, want /app/data", vol.MountPath)
	}
	if vol.VolumeType != "nfs" {
		t.Errorf("volumeType = %q, want nfs", vol.VolumeType)
	}
	if !vol.Temporary {
		t.Error("expected temporary=true")
	}
}
