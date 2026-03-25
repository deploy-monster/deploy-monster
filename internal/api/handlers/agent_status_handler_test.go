package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// testCoreForAgent returns a minimal *core.Core with Registry and Build for agent tests.
func testCoreForAgent() *core.Core {
	return &core.Core{
		Config: &core.Config{},
		Build: core.BuildInfo{
			Version: "1.0.0-test",
			Commit:  "abc123",
			Date:    "2025-01-01",
		},
		Registry: core.NewRegistry(),
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
}

// ─── List Agent Status ───────────────────────────────────────────────────────

func TestAgentStatus_List_Success(t *testing.T) {
	handler := NewAgentStatusHandler(testCoreForAgent())

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
	if local["status"] != "connected" {
		t.Errorf("expected status=connected, got %v", local["status"])
	}
}

// ─── Get Agent ───────────────────────────────────────────────────────────────

func TestAgentStatus_GetAgent_Success(t *testing.T) {
	handler := NewAgentStatusHandler(testCoreForAgent())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/server-42", nil)
	req.SetPathValue("id", "server-42")
	rr := httptest.NewRecorder()

	handler.GetAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["server_id"] != "server-42" {
		t.Errorf("expected server_id=server-42, got %v", resp["server_id"])
	}
}
