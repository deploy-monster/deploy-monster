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

func TestTopologyHandler_Deploy_NoClaims(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_InvalidBody(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader([]byte(`{invalid`)))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_EmptyNodes(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes:       []TopologyNode{},
		ProjectID:   "proj-1",
		Environment: "production",
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_PathTraversal(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		ProjectID:   "../etc/passwd",
		Environment: "production",
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_EncodedPathTraversal(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		ProjectID:   "%252e%252e%252fetc",
		Environment: "production",
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestValidateTopologyPathParts(t *testing.T) {
	tests := []struct {
		name        string
		projectID   string
		environment string
		want        bool
	}{
		{name: "valid", projectID: "proj-1", environment: "production", want: true},
		{name: "underscore", projectID: "proj_1", environment: "staging_2", want: true},
		{name: "empty project", projectID: "", environment: "production", want: false},
		{name: "dot traversal", projectID: "../etc", environment: "production", want: false},
		{name: "encoded traversal", projectID: "%2e%2e%2fetc", environment: "production", want: false},
		{name: "double encoded traversal", projectID: "%252e%252e%252fetc", environment: "production", want: false},
		{name: "absolute path", projectID: "/var/lib", environment: "production", want: false},
		{name: "backslash path", projectID: "proj-1", environment: "..\\prod", want: false},
		{name: "whitespace", projectID: " proj-1", environment: "production", want: false},
		{name: "colon", projectID: "proj:1", environment: "production", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateTopologyPathParts(tt.projectID, tt.environment); got != tt.want {
				t.Fatalf("validateTopologyPathParts(%q, %q) = %v, want %v", tt.projectID, tt.environment, got, tt.want)
			}
		})
	}
}

func TestTopologyHandler_Deploy_CrossTenantProject(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj-1", TenantID: "tenant-2", Name: "Foreign Project"})
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes: []TopologyNode{{
			ID:   "app-1",
			Type: "app",
			Data: map[string]any{
				"name":   "api",
				"gitUrl": "https://github.com/user/api",
				"branch": "main",
				"port":   3000,
			},
		}},
		ProjectID:   "proj-1",
		Environment: "production",
		DryRun:      true,
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
