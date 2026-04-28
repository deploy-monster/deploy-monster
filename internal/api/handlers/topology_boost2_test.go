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

func TestTopologyHandler_Save_Success(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
}

func TestTopologyHandler_Save_NoClaims(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Save_InvalidBody(t *testing.T) {
	store := newMockStore()
	bolt := newMockBoltStore()
	c := &core.Core{DB: &core.Database{Bolt: bolt}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader([]byte(`{invalid`)))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetStringMapFromMap(t *testing.T) {
	m := map[string]any{
		"labels": map[string]any{
			"app":  "myapp",
			"env":  "prod",
			"num":  42,
		},
		"empty": map[string]any{},
	}

	result := getStringMapFromMap(m, "labels")
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
	if result["app"] != "myapp" {
		t.Errorf("app = %q, want myapp", result["app"])
	}
	if result["env"] != "prod" {
		t.Errorf("env = %q, want prod", result["env"])
	}
	// Non-string value should be skipped
	if _, ok := result["num"]; ok {
		t.Error("expected num to be skipped (not a string)")
	}

	// Missing key
	empty := getStringMapFromMap(m, "missing")
	if len(empty) != 0 {
		t.Errorf("expected empty map for missing key, got %d", len(empty))
	}

	// Key exists but not a map[string]any
	notMap := getStringMapFromMap(m, "num")
	if len(notMap) != 0 {
		t.Errorf("expected empty map for non-map value, got %d", len(notMap))
	}
}
