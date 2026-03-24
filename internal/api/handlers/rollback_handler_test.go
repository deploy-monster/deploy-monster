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

// ─── Rollback To Commit ─────────────────────────────────────────────────────

func TestRollbackToCommit_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "testapp", TenantID: "t1", Status: "running"})
	store.addDeployment("app1", core.Deployment{
		ID:        "dep1",
		AppID:     "app1",
		Version:   1,
		CommitSHA: "abc1234567890def",
		Image:     "myapp:v1",
		Status:    "success",
	})
	store.addDeployment("app1", core.Deployment{
		ID:        "dep2",
		AppID:     "app1",
		Version:   2,
		CommitSHA: "def4567890abcdef",
		Image:     "myapp:v2",
		Status:    "running",
	})

	handler := NewCommitRollbackHandler(store, &mockContainerRuntime{}, testCore().Events)

	body, _ := json.Marshal(commitRollbackRequest{CommitSHA: "def4567890abcdef"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rollback-to-commit", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.RollbackToCommit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["commit"] != "def4567890abcdef" {
		t.Errorf("expected commit=def4567890abcdef, got %v", resp["commit"])
	}
	if int(resp["version"].(float64)) != 2 {
		t.Errorf("expected version=2, got %v", resp["version"])
	}
}

func TestRollbackToCommit_PartialSHA(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "testapp", TenantID: "t1", Status: "running"})
	store.addDeployment("app1", core.Deployment{
		ID:        "dep1",
		AppID:     "app1",
		Version:   1,
		CommitSHA: "abc1234567890def",
		Image:     "myapp:v1",
		Status:    "success",
	})

	handler := NewCommitRollbackHandler(store, &mockContainerRuntime{}, testCore().Events)

	body, _ := json.Marshal(commitRollbackRequest{CommitSHA: "abc1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rollback-to-commit", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.RollbackToCommit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if int(resp["version"].(float64)) != 1 {
		t.Errorf("expected version=1, got %v", resp["version"])
	}
}

func TestRollbackToCommit_NotFound(t *testing.T) {
	store := newMockStore()
	store.addDeployment("app1", core.Deployment{
		ID:        "dep1",
		AppID:     "app1",
		Version:   1,
		CommitSHA: "abc1234567890def",
	})

	handler := NewCommitRollbackHandler(store, &mockContainerRuntime{}, testCore().Events)

	body, _ := json.Marshal(commitRollbackRequest{CommitSHA: "ffffffffffffffff"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rollback-to-commit", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.RollbackToCommit(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "no deployment found for commit ffffffffffffffff")
}

func TestRollbackToCommit_EmptyCommitSHA(t *testing.T) {
	store := newMockStore()
	handler := NewCommitRollbackHandler(store, &mockContainerRuntime{}, testCore().Events)

	body, _ := json.Marshal(commitRollbackRequest{CommitSHA: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rollback-to-commit", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.RollbackToCommit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "commit_sha required")
}

func TestRollbackToCommit_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewCommitRollbackHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rollback-to-commit", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.RollbackToCommit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestRollbackToCommit_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListDeploymentsByApp = errors.New("db error")

	handler := NewCommitRollbackHandler(store, &mockContainerRuntime{}, testCore().Events)

	body, _ := json.Marshal(commitRollbackRequest{CommitSHA: "abc1234567890def"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/rollback-to-commit", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.RollbackToCommit(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
