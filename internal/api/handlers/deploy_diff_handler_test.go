package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Deploy Diff ─────────────────────────────────────────────────────────────

func TestDeployDiff_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	store.addDeployment("app1", core.Deployment{
		ID:          "dep1",
		AppID:       "app1",
		Version:     1,
		Image:       "myapp:v1",
		CommitSHA:   "abc123",
		Strategy:    "recreate",
		TriggeredBy: "user",
	})
	store.addDeployment("app1", core.Deployment{
		ID:          "dep2",
		AppID:       "app1",
		Version:     2,
		Image:       "myapp:v2",
		CommitSHA:   "def456",
		Strategy:    "rolling",
		TriggeredBy: "webhook",
	})

	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff?from=1&to=2", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if int(resp["from"].(float64)) != 1 {
		t.Errorf("expected from=1, got %v", resp["from"])
	}
	if int(resp["to"].(float64)) != 2 {
		t.Errorf("expected to=2, got %v", resp["to"])
	}

	changes, ok := resp["changes"].(map[string]any)
	if !ok {
		t.Fatal("expected changes object in response")
	}

	imageDiff := changes["image"].(map[string]any)
	if imageDiff["from"] != "myapp:v1" {
		t.Errorf("expected image.from=myapp:v1, got %v", imageDiff["from"])
	}
	if imageDiff["to"] != "myapp:v2" {
		t.Errorf("expected image.to=myapp:v2, got %v", imageDiff["to"])
	}

	commitDiff := changes["commit"].(map[string]any)
	if commitDiff["from"] != "abc123" {
		t.Errorf("expected commit.from=abc123, got %v", commitDiff["from"])
	}
	if commitDiff["to"] != "def456" {
		t.Errorf("expected commit.to=def456, got %v", commitDiff["to"])
	}

	strategyDiff := changes["strategy"].(map[string]any)
	if strategyDiff["from"] != "recreate" {
		t.Errorf("expected strategy.from=recreate, got %v", strategyDiff["from"])
	}
	if strategyDiff["to"] != "rolling" {
		t.Errorf("expected strategy.to=rolling, got %v", strategyDiff["to"])
	}

	triggerDiff := changes["triggered_by"].(map[string]any)
	if triggerDiff["from"] != "user" {
		t.Errorf("expected triggered_by.from=user, got %v", triggerDiff["from"])
	}
	if triggerDiff["to"] != "webhook" {
		t.Errorf("expected triggered_by.to=webhook, got %v", triggerDiff["to"])
	}
}

func TestDeployDiff_MissingVersionParams(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "from and to version numbers required")
}

func TestDeployDiff_MissingFromParam(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff?to=2", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "from and to version numbers required")
}

func TestDeployDiff_MissingToParam(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff?from=1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "from and to version numbers required")
}

func TestDeployDiff_VersionNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	store.addDeployment("app1", core.Deployment{
		ID:      "dep1",
		AppID:   "app1",
		Version: 1,
		Image:   "myapp:v1",
	})

	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff?from=1&to=99", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "one or both versions not found")
}

func TestDeployDiff_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	store.errListDeploymentsByApp = errors.New("db error")

	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff?from=1&to=2", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "internal error")
}

func TestDeployDiff_InvalidVersionStrings(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	handler := NewDeployDiffHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deployments/diff?from=abc&to=xyz", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Diff(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "from and to version numbers required")
}
