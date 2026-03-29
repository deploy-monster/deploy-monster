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

func TestTopologyHandler_Save(t *testing.T) {
	store := newMockStore()
	c := &core.Core{}
	h := NewTopologyHandler(store, c)

	// Create valid request
	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{
				ID:   "app-1",
				Type: "app",
				Position: Position{X: 100, Y: 100},
				Data: map[string]interface{}{
					"name":     "my-app",
					"gitUrl":   "https://github.com/user/repo",
					"branch":   "main",
					"port":     3000,
					"replicas": 1,
				},
			},
		},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	// Add auth claims
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if success, ok := resp["success"].(bool); !ok || !success {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
}

func TestTopologyHandler_Deploy(t *testing.T) {
	store := newMockStore()
	c := &core.Core{}
	h := NewTopologyHandler(store, c)

	// Create deploy request with multiple node types
	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{
				ID:   "db-1",
				Type: "database",
				Position: Position{X: 100, Y: 100},
				Data: map[string]interface{}{
					"name":    "postgres-main",
					"engine":  "postgres",
					"version": "16",
					"sizeGB":  10,
				},
			},
			{
				ID:   "app-1",
				Type: "app",
				Position: Position{X: 350, Y: 100},
				Data: map[string]interface{}{
					"name":     "api-server",
					"gitUrl":   "https://github.com/user/api",
					"branch":   "main",
					"port":     3000,
					"replicas": 2,
				},
			},
			{
				ID:   "domain-1",
				Type: "domain",
				Position: Position{X: 600, Y: 100},
				Data: map[string]interface{}{
					"name":        "api.example.com",
					"fqdn":        "api.example.com",
					"sslEnabled":  true,
				},
			},
		},
		Edges: []TopologyEdge{
			{
				ID:     "edge-db-app",
				Source: "db-1",
				Target: "app-1",
				Type:   "dependency",
			},
			{
				ID:     "edge-app-domain",
				Source: "app-1",
				Target: "domain-1",
				Type:   "dns",
			},
		},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	// Add auth claims
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TopologyDeployResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success=true, got false: %s", resp.Message)
	}

	if resp.CreatedResources == nil {
		t.Fatal("expected createdResources to be set")
	}

	// Check that resources were created
	if len(resp.CreatedResources.Databases) != 1 {
		t.Errorf("expected 1 database, got %d", len(resp.CreatedResources.Databases))
	}
	if len(resp.CreatedResources.Apps) != 1 {
		t.Errorf("expected 1 app, got %d", len(resp.CreatedResources.Apps))
	}
	if len(resp.CreatedResources.Domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(resp.CreatedResources.Domains))
	}
}

func TestTopologyHandler_DeployEmptyNodes(t *testing.T) {
	store := newMockStore()
	c := &core.Core{}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes:       []TopologyNode{},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	// Add auth claims
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTopologyHandler_DeployWorkerNode(t *testing.T) {
	store := newMockStore()
	c := &core.Core{}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{
				ID:   "worker-1",
				Type: "worker",
				Position: Position{X: 100, Y: 100},
				Data: map[string]interface{}{
					"name":     "background-worker",
					"gitUrl":   "https://github.com/user/worker",
					"branch":   "main",
					"command":  "npm run worker",
					"replicas": 2,
				},
			},
		},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TopologyDeployResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Workers should be tracked as apps
	if len(resp.CreatedResources.Apps) != 1 {
		t.Errorf("expected 1 app (worker), got %d", len(resp.CreatedResources.Apps))
	}
}
