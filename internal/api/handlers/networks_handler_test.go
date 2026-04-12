package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── List Networks ───────────────────────────────────────────────────────────

func TestListNetworks_Success(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "container1abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.stack":  "web",
				},
			},
			{
				ID:    "container2abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.stack":  "api",
				},
			},
		},
	}

	handler := NewNetworkHandler(runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	// monster-web-net, monster-api-net, monster-network (always present)
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array")
	}

	// Verify the default network is always present.
	hasDefault := false
	for _, n := range data {
		if n.(string) == "monster-network" {
			hasDefault = true
		}
	}
	if !hasDefault {
		t.Error("expected 'monster-network' to be in the list")
	}
}

func TestListNetworks_NilRuntime(t *testing.T) {
	handler := NewNetworkHandler(nil, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
}

func TestListNetworks_NoStacks(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "container1abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					// No stack label
				},
			},
		},
	}

	handler := NewNetworkHandler(runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	// Only monster-network should be present.
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
}

func TestListNetworks_DuplicateStacks(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "container1abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.stack":  "web",
				},
			},
			{
				ID:    "container2abcdef",
				State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.stack":  "web", // duplicate
				},
			},
		},
	}

	handler := NewNetworkHandler(runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total := int(resp["total"].(float64))
	// monster-web-net + monster-network = 2
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
}

// ─── Connect Network ─────────────────────────────────────────────────────────

func TestConnectNetwork_Success(t *testing.T) {
	handler := NewNetworkHandler(nil, testCore().Events)

	body, _ := json.Marshal(connectNetworkRequest{
		ContainerID: "abc123",
		Network:     "monster-network",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/networks/connect", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Connect(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "connected" {
		t.Errorf("expected status=connected, got %q", resp["status"])
	}
	if resp["container"] != "abc123" {
		t.Errorf("expected container=abc123, got %q", resp["container"])
	}
	if resp["network"] != "monster-network" {
		t.Errorf("expected network=monster-network, got %q", resp["network"])
	}
}

func TestConnectNetwork_InvalidJSON(t *testing.T) {
	handler := NewNetworkHandler(nil, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/networks/connect", bytes.NewReader([]byte("{")))
	rr := httptest.NewRecorder()

	handler.Connect(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}
