package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── List Agent Status ───────────────────────────────────────────────────────

func TestAgentStatus_List_Success(t *testing.T) {
	handler := NewAgentStatusHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

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

	local, ok := resp["local"].(map[string]any)
	if !ok {
		t.Fatal("expected local agent info in response")
	}
	if local["server_id"] != "local" {
		t.Errorf("expected server_id=local, got %v", local["server_id"])
	}
	if local["hostname"] != "localhost" {
		t.Errorf("expected hostname=localhost, got %v", local["hostname"])
	}
	if local["status"] != "connected" {
		t.Errorf("expected status=connected, got %v", local["status"])
	}
}

// ─── Get Agent ───────────────────────────────────────────────────────────────

func TestAgentStatus_GetAgent_Success(t *testing.T) {
	handler := NewAgentStatusHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/server-42", nil)
	req.SetPathValue("id", "server-42")
	rr := httptest.NewRecorder()

	handler.GetAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var status AgentNodeStatus
	json.Unmarshal(rr.Body.Bytes(), &status)

	if status.ServerID != "server-42" {
		t.Errorf("expected server_id=server-42, got %q", status.ServerID)
	}
	if status.Status != "unknown" {
		t.Errorf("expected status=unknown, got %q", status.Status)
	}
	if status.LastSeen.IsZero() {
		t.Error("expected non-zero last_seen")
	}
}
