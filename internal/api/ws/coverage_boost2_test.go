package ws

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Hub() — covers the previously 0.0% covered function
// =============================================================================

func TestHub_ReturnsGlobal(t *testing.T) {
	h := Hub()
	if h == nil {
		t.Fatal("Hub() returned nil")
	}
	if h != deployHub {
		t.Error("Hub() should return the global deployHub")
	}
}

// =============================================================================
// writeSSEData — covers the empty data branch (line 19-20)
// =============================================================================

func TestWriteSSEData_Empty(t *testing.T) {
	var buf strings.Builder
	writeSSEData(&buf, "")
	if buf.String() != "data: \n" {
		t.Errorf("unexpected output for empty data: %q", buf.String())
	}
}

func TestWriteSSEData_Multiline(t *testing.T) {
	var buf strings.Builder
	writeSSEData(&buf, "line1\nline2\nline3\r")
	output := buf.String()
	if !strings.Contains(output, "data: line1") {
		t.Errorf("expected data: line1, got %q", output)
	}
	if !strings.Contains(output, "data: line2") {
		t.Errorf("expected data: line2, got %q", output)
	}
	if strings.Contains(output, "\r") {
		t.Errorf("output should not contain \\r: %q", output)
	}
}

// =============================================================================
// DeployHub — snapshotClients for closed hub
// =============================================================================

func TestSnapshotClients_ClosedHub(t *testing.T) {
	h := NewDeployHub()
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()

	snapshot := h.snapshotClients("project-1")
	if snapshot != nil {
		t.Error("expected nil snapshot for closed hub")
	}
}

// =============================================================================
// DeployHub — Unregister with non-existent project or conn
// =============================================================================

func TestUnregister_NonexistentProject(t *testing.T) {
	h := NewDeployHub()
	h.Unregister("nonexistent", nil)
}

func TestUnregister_NonexistentConn(t *testing.T) {
	h := NewDeployHub()
	h.mu.Lock()
	h.clients["project-1"] = make(map[*websocket.Conn]*clientConn)
	h.mu.Unlock()
	h.Unregister("project-1", nil)
}

// =============================================================================
// Broadcaster — dead client eviction in BroadcastComplete/Progress
// =============================================================================

func TestBroadcastComplete_NoClients(t *testing.T) {
	h := NewDeployHub()
	h.BroadcastComplete("empty-project", true, "done", "1s", nil, nil, nil, nil)
}

func TestBroadcastProgress_NoClients(t *testing.T) {
	h := NewDeployHub()
	h.BroadcastProgress("empty-project", "build", "building", 50)
}

// =============================================================================
// StreamLogs — runtime nil, no container, error paths
// =============================================================================

func TestStreamLogs_NilRuntime(t *testing.T) {
	ls := &LogStreamer{runtime: nil, logger: discardLogger()}
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/logs/stream?tail=50", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()
	ls.StreamLogs(w, req)
}

func TestStreamLogs_NoContainer(t *testing.T) {
	ls := &LogStreamer{
		runtime: &mockRuntime{containers: []core.ContainerInfo{}},
		logger:  discardLogger(),
	}
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/logs/stream?tail=50", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()
	ls.StreamLogs(w, req)
}

func TestStreamLogs_LogsError(t *testing.T) {
	ls := &LogStreamer{
		runtime: &mockRuntime{
			containers: []core.ContainerInfo{
				{ID: "c1", Name: "app1"},
			},
			logsErr: fmt.Errorf("log error"),
		},
		logger: discardLogger(),
	}
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/logs/stream?tail=50", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()
	ls.StreamLogs(w, req)
}

// =============================================================================
// StreamEvents — nil event bus
// =============================================================================

func TestStreamEvents_NilBus(t *testing.T) {
	es := &EventStreamer{events: nil, logger: discardLogger()}
	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	w := httptest.NewRecorder()
	es.StreamEvents(w, req)
}

// =============================================================================
// Terminal — SendCommand with nil store (skips app lookup)
// =============================================================================

func TestSendCommand_NilStore(t *testing.T) {
	term := &Terminal{
		runtime: &mockRuntime{
			containers: []core.ContainerInfo{
				{ID: "c1", Name: "app1"},
			},
			execOutput: "ok",
		},
		store:  nil,
		logger: discardLogger(),
	}
	body := strings.NewReader(`{"command":"ls -la"}`)
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()
	term.SendCommand(w, req)
}

func TestSendCommand_RuntimeError(t *testing.T) {
	term := &Terminal{
		runtime: &mockRuntime{
			listErr: fmt.Errorf("list error"),
		},
		store:  &mockStore{app: &core.Application{ID: "app-1"}},
		logger: discardLogger(),
	}
	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()
	term.SendCommand(w, req)
}

// =============================================================================
// writeSSEComment — covers the SSE comment function
// =============================================================================

func TestWriteSSEComment(t *testing.T) {
	var buf strings.Builder
	writeSSEComment(&buf, "keepalive")
	output := buf.String()
	if !strings.Contains(output, ": keepalive") {
		t.Errorf("expected ': keepalive' in output, got %q", output)
	}
}

// =============================================================================
// shortResourceID — covers both id length cases
// =============================================================================

func TestShortResourceID_ShortID(t *testing.T) {
	result := shortResourceID("abc123")
	if result != "abc123" {
		t.Errorf("shortResourceID('abc123') = %q, want 'abc123'", result)
	}
}

func TestShortResourceID_LongID(t *testing.T) {
	result := shortResourceID("abcdef1234567890")
	if result != "abcdef123456" {
		t.Errorf("shortResourceID('abcdef1234567890') = %q, want 'abcdef123456'", result)
	}
}

// =============================================================================
// GetDeployHub — deprecated accessor
// =============================================================================

func TestGetDeployHub(t *testing.T) {
	h := GetDeployHub()
	if h == nil {
		t.Fatal("GetDeployHub() returned nil")
	}
}

// =============================================================================
// DeployHub — Enter on closed hub
// =============================================================================

func TestEnter_ClosedHub(t *testing.T) {
	h := NewDeployHub()
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	if h.enter() {
		t.Error("enter() should return false for closed hub")
	}
}

// =============================================================================
// DeployHub — Register on closed hub
// =============================================================================

func TestRegister_ClosedHub(t *testing.T) {
	h := NewDeployHub()
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()

	cc := h.Register("project-1", nil)
	if cc != nil {
		t.Error("Register() should return nil for closed hub")
	}
}

// =============================================================================
// decodeTerminalCommand — non-JSON body
// =============================================================================

func TestDecodeTerminalCommand_InvalidJSON(t *testing.T) {
	body := strings.NewReader(`not json`)
	r := httptest.NewRequest("POST", "/", body)
	var target struct {
		Command string `json:"command"`
	}
	if decodeTerminalCommand(r, &target) {
		t.Error("expected false for invalid JSON")
	}
}
