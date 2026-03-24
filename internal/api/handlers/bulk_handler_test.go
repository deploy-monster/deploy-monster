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

// ─── Bulk Execute ────────────────────────────────────────────────────────────

func TestBulkExecute_Start(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "start",
		AppIDs: []string{"app1", "app2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}

	succeeded := int(resp["succeeded"].(float64))
	if succeeded != 2 {
		t.Errorf("expected succeeded=2, got %d", succeeded)
	}

	failed := int(resp["failed"].(float64))
	if failed != 0 {
		t.Errorf("expected failed=0, got %d", failed)
	}

	results := resp["results"].([]any)
	for _, r := range results {
		result := r.(map[string]any)
		if result["status"] != "started" {
			t.Errorf("expected status 'started', got %q", result["status"])
		}
	}

	if store.updatedStatus["app1"] != "running" {
		t.Errorf("expected app1 status 'running', got %q", store.updatedStatus["app1"])
	}
	if store.updatedStatus["app2"] != "running" {
		t.Errorf("expected app2 status 'running', got %q", store.updatedStatus["app2"])
	}
}

func TestBulkExecute_Stop(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "stop",
		AppIDs: []string{"app1"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	results := resp["results"].([]any)
	result := results[0].(map[string]any)
	if result["status"] != "stopped" {
		t.Errorf("expected status 'stopped', got %q", result["status"])
	}

	if store.updatedStatus["app1"] != "stopped" {
		t.Errorf("expected app1 status 'stopped', got %q", store.updatedStatus["app1"])
	}
}

func TestBulkExecute_Restart(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "restart",
		AppIDs: []string{"app1"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	results := resp["results"].([]any)
	result := results[0].(map[string]any)
	if result["status"] != "restarted" {
		t.Errorf("expected status 'restarted', got %q", result["status"])
	}
}

func TestBulkExecute_Delete(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "delete",
		AppIDs: []string{"app1"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	results := resp["results"].([]any)
	result := results[0].(map[string]any)
	if result["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %q", result["status"])
	}

	if store.deletedAppID != "app1" {
		t.Errorf("expected deleted app ID 'app1', got %q", store.deletedAppID)
	}
}

func TestBulkExecute_UnknownAction(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "explode",
		AppIDs: []string{"app1"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	failed := int(resp["failed"].(float64))
	if failed != 1 {
		t.Errorf("expected failed=1, got %d", failed)
	}

	results := resp["results"].([]any)
	result := results[0].(map[string]any)
	if result["status"] != "error" {
		t.Errorf("expected status 'error', got %q", result["status"])
	}
}

func TestBulkExecute_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{Action: "start", AppIDs: []string{"app1"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestBulkExecute_InvalidJSON(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader([]byte("{")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestBulkExecute_MissingAction(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{AppIDs: []string{"app1"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "action and app_ids are required")
}

func TestBulkExecute_EmptyAppIDs(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{Action: "start", AppIDs: []string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "action and app_ids are required")
}

func TestBulkExecute_TooManyApps(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	ids := make([]string, 51)
	for i := range ids {
		ids[i] = core.GenerateID()
	}

	body, _ := json.Marshal(bulkRequest{Action: "start", AppIDs: ids})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "max 50 apps per bulk operation")
}

func TestBulkExecute_PartialFailure(t *testing.T) {
	store := newMockStore()
	store.errUpdateAppStatus = errors.New("db error")

	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "start",
		AppIDs: []string{"app1", "app2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	failed := int(resp["failed"].(float64))
	if failed != 2 {
		t.Errorf("expected failed=2 (all failed due to store error), got %d", failed)
	}
}
