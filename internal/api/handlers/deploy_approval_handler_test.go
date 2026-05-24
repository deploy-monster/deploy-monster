package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Deploy Approval ─────────────────────────────────────────────────────────

func TestDeployApproval_ListPending_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deploy/approvals", nil)
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.ListPending(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

func TestDeployApproval_ListPending_NoClaims(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deploy/approvals", nil)
	rr := httptest.NewRecorder()

	handler.ListPending(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDeployApproval_ListPending_TenantScoped(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)
	handler.pending["appr1"] = &ApprovalRequest{ID: "appr1", AppID: "app1", TenantID: "tenant1", Status: "pending"}
	handler.pending["appr2"] = &ApprovalRequest{ID: "appr2", AppID: "app2", TenantID: "tenant2", Status: "pending"}
	handler.pending["appr3"] = &ApprovalRequest{ID: "appr3", AppID: "app3", TenantID: "tenant1", Status: "approved"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deploy/approvals", nil)
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.ListPending(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Fatalf("expected one tenant-local pending approval, got %v", resp["total"])
	}
	data := resp["data"].([]any)
	item := data[0].(map[string]any)
	if item["id"] != "appr1" {
		t.Fatalf("listed approval id = %v, want appr1", item["id"])
	}
}

func TestDeployApproval_Approve_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)

	// Pre-populate a pending approval request so Approve can find it.
	handler.pending["appr1"] = &ApprovalRequest{
		ID:       "appr1",
		AppID:    "app1",
		TenantID: "tenant1", // Must match the tenant from withClaims
		Status:   "pending",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals/appr1/approve", nil)
	req.SetPathValue("id", "appr1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Approve(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["approval_id"] != "appr1" {
		t.Errorf("expected approval_id=appr1, got %q", resp["approval_id"])
	}
	if resp["status"] != "approved" {
		t.Errorf("expected status=approved, got %q", resp["status"])
	}
	if resp["approved_by"] != "user1" {
		t.Errorf("expected approved_by=user1, got %q", resp["approved_by"])
	}
}

func TestDeployApproval_Approve_AlreadyProcessedConflict(t *testing.T) {
	handler := NewDeployApprovalHandler(newMockStore(), nil)
	handler.pending["appr1"] = &ApprovalRequest{
		ID:         "appr1",
		AppID:      "app1",
		TenantID:   "tenant1",
		Status:     "rejected",
		ReviewedBy: "reviewer1",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals/appr1/approve", nil)
	req.SetPathValue("id", "appr1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Approve(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := handler.pending["appr1"].Status; got != "rejected" {
		t.Fatalf("status changed to %q, want rejected", got)
	}
}

func TestDeployApproval_Approve_NoClaims(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals/appr1/approve", nil)
	req.SetPathValue("id", "appr1")
	rr := httptest.NewRecorder()

	handler.Approve(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestDeployApproval_Reject_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)

	// Pre-populate a pending approval request so Reject can find it.
	handler.pending["appr1"] = &ApprovalRequest{
		ID:       "appr1",
		AppID:    "app1",
		TenantID: "tenant1",
		Status:   "pending",
	}

	body, _ := json.Marshal(map[string]string{"reason": "not ready for production"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals/appr1/reject", bytes.NewReader(body))
	req.SetPathValue("id", "appr1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Reject(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["approval_id"] != "appr1" {
		t.Errorf("expected approval_id=appr1, got %q", resp["approval_id"])
	}
	if resp["status"] != "rejected" {
		t.Errorf("expected status=rejected, got %q", resp["status"])
	}
	if resp["reason"] != "not ready for production" {
		t.Errorf("expected reason='not ready for production', got %q", resp["reason"])
	}
}

func TestDeployApproval_Reject_AlreadyProcessedConflict(t *testing.T) {
	handler := NewDeployApprovalHandler(newMockStore(), nil)
	handler.pending["appr1"] = &ApprovalRequest{
		ID:         "appr1",
		AppID:      "app1",
		TenantID:   "tenant1",
		Status:     "approved",
		ReviewedBy: "reviewer1",
	}

	body, _ := json.Marshal(map[string]string{"reason": "too late"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals/appr1/reject", bytes.NewReader(body))
	req.SetPathValue("id", "appr1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Reject(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := handler.pending["appr1"].Status; got != "approved" {
		t.Fatalf("status changed to %q, want approved", got)
	}
}

func TestDeployApproval_Reject_NoReason(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployApprovalHandler(store, events)

	// Pre-populate a pending approval request so Reject can find it.
	handler.pending["appr1"] = &ApprovalRequest{
		ID:       "appr1",
		AppID:    "app1",
		TenantID: "tenant1",
		Status:   "pending",
	}

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/approvals/appr1/reject", bytes.NewReader(body))
	req.SetPathValue("id", "appr1")
	req = withClaims(req, "user1", "tenant1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Reject(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "rejected" {
		t.Errorf("expected status=rejected, got %q", resp["status"])
	}
	if resp["reason"] != "" {
		t.Errorf("expected empty reason, got %q", resp["reason"])
	}
}
