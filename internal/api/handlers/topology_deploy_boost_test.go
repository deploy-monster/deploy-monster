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
