package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Deploy Notify ───────────────────────────────────────────────────────────

func TestDeployNotify_Get_Success(t *testing.T) {
	store := newMockStore()
	handler := NewDeployNotifyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deploy-notifications", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp DeployNotifyConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.OnSuccess != nil {
		t.Errorf("expected nil on_success, got %v", resp.OnSuccess)
	}
	if resp.OnFailure != nil {
		t.Errorf("expected nil on_failure, got %v", resp.OnFailure)
	}
	if resp.OnRollback != nil {
		t.Errorf("expected nil on_rollback, got %v", resp.OnRollback)
	}
}

func TestDeployNotify_Update_Success(t *testing.T) {
	store := newMockStore()
	handler := NewDeployNotifyHandler(store, newMockBoltStore())

	body, _ := json.Marshal(DeployNotifyConfig{
		OnSuccess: []NotifyTarget{
			{Channel: "slack", Recipient: "#deployments"},
		},
		OnFailure: []NotifyTarget{
			{Channel: "email", Recipient: "ops@company.com"},
			{Channel: "discord", Recipient: "alerts-channel"},
		},
		OnRollback: []NotifyTarget{
			{Channel: "telegram", Recipient: "@ops_bot"},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/deploy-notifications", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}

	cfg, ok := resp["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config object in response")
	}

	onSuccess, ok := cfg["on_success"].([]any)
	if !ok {
		t.Fatal("expected on_success array in config")
	}
	if len(onSuccess) != 1 {
		t.Errorf("expected 1 on_success target, got %d", len(onSuccess))
	}

	onFailure := cfg["on_failure"].([]any)
	if len(onFailure) != 2 {
		t.Errorf("expected 2 on_failure targets, got %d", len(onFailure))
	}

	onRollback := cfg["on_rollback"].([]any)
	if len(onRollback) != 1 {
		t.Errorf("expected 1 on_rollback target, got %d", len(onRollback))
	}
}

func TestDeployNotify_Update_EmptyConfig(t *testing.T) {
	store := newMockStore()
	handler := NewDeployNotifyHandler(store, newMockBoltStore())

	body, _ := json.Marshal(DeployNotifyConfig{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/deploy-notifications", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}
}

func TestDeployNotify_Update_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewDeployNotifyHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/app1/deploy-notifications", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}
