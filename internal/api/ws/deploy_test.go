package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewDeployHub(t *testing.T) {
	hub := NewDeployHub()
	if hub == nil {
		t.Fatal("NewDeployHub() returned nil")
	}
	if hub.clients == nil {
		t.Fatal("clients map should be initialized")
	}
}

func TestDeployHub_RegisterUnregister(t *testing.T) {
	hub := NewDeployHub()
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		hub.Register("proj-1", conn)

		hub.mu.RLock()
		count := len(hub.clients["proj-1"])
		hub.mu.RUnlock()

		if count != 1 {
			t.Errorf("expected 1 client after Register, got %d", count)
		}

		hub.Unregister("proj-1", conn)

		hub.mu.RLock()
		_, exists := hub.clients["proj-1"]
		hub.mu.RUnlock()

		if exists {
			t.Error("project should be removed after last client unregisters")
		}

		conn.Close()
		close(done)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out")
	}
}

func TestDeployHub_BroadcastProgress(t *testing.T) {
	hub := NewDeployHub()
	ready := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hub.Register("proj-1", conn)
		defer hub.Unregister("proj-1", conn)

		close(ready) // signal that registration is complete

		// Keep alive until client disconnects
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Wait for server-side registration
	<-ready

	// Broadcast — the client should receive the message
	hub.BroadcastProgress("proj-1", "building", "Building image", 50)

	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var msg DeployProgressMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if msg.Type != "deploy_progress" {
		t.Errorf("Type = %q, want %q", msg.Type, "deploy_progress")
	}
	if msg.Stage != "building" {
		t.Errorf("Stage = %q, want %q", msg.Stage, "building")
	}
	if msg.Progress != 50 {
		t.Errorf("Progress = %d, want 50", msg.Progress)
	}
	if msg.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", msg.ProjectID, "proj-1")
	}
}

func TestDeployHub_BroadcastComplete(t *testing.T) {
	hub := NewDeployHub()
	ready := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hub.Register("proj-2", conn)
		defer hub.Unregister("proj-2", conn)

		close(ready)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	<-ready

	hub.BroadcastComplete("proj-2", true, "Done", "5s",
		[]string{"c1"}, []string{"n1"}, []string{"v1"}, nil)

	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var msg DeployCompleteMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if msg.Type != "deploy_complete" {
		t.Errorf("Type = %q, want %q", msg.Type, "deploy_complete")
	}
	if !msg.Success {
		t.Error("expected Success = true")
	}
	if msg.Duration != "5s" {
		t.Errorf("Duration = %q, want %q", msg.Duration, "5s")
	}
	if len(msg.Containers) != 1 || msg.Containers[0] != "c1" {
		t.Errorf("Containers = %v", msg.Containers)
	}
}

func TestDeployHub_BroadcastProgress_NoClients_NoPanic(t *testing.T) {
	hub := NewDeployHub()
	// Should not panic when no clients are registered
	hub.BroadcastProgress("unknown-project", "building", "test", 50)
	hub.BroadcastComplete("unknown-project", false, "err", "", nil, nil, nil, []string{"fail"})
}

func TestGetDeployHub_ReturnsSingleton(t *testing.T) {
	h1 := GetDeployHub()
	h2 := GetDeployHub()
	if h1 != h2 {
		t.Error("GetDeployHub should return the same instance")
	}
}

func TestServeWS_CleansUpOnDisconnect(t *testing.T) {
	hub := NewDeployHub()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-cleanup")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Verify the client was registered
	time.Sleep(50 * time.Millisecond)
	hub.mu.RLock()
	count := len(hub.clients["proj-cleanup"])
	hub.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 client, got %d", count)
	}

	// Close the connection — triggers cleanup of both read loop and ping goroutine
	conn.Close()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)
	hub.mu.RLock()
	count = len(hub.clients["proj-cleanup"])
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients after close, got %d", count)
	}
}

func TestDeployHub_OriginValidation_AllowAll(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("*")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-origin")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	header := http.Header{}
	header.Set("Origin", "http://evil.example.com")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("expected connection with * origins, got error: %v", err)
	}
	conn.Close()
}

func TestDeployHub_OriginValidation_AllowedOrigin(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("http://app.example.com, http://admin.example.com")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-origin2")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	header := http.Header{}
	header.Set("Origin", "http://admin.example.com")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("expected connection for allowed origin, got error: %v", err)
	}
	conn.Close()
}

func TestDeployHub_OriginValidation_RejectedOrigin(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("http://app.example.com")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-origin3")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	header := http.Header{}
	header.Set("Origin", "http://evil.example.com")
	_, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Fatal("expected connection to be rejected for disallowed origin")
	}
}

func TestDeployHub_OriginValidation_NoOriginHeader(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("http://app.example.com")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-origin4")
	}))
	defer server.Close()

	// No Origin header — security fix: empty origin is rejected
	// This prevents cross-origin WebSocket connections from tools that don't send Origin headers
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		conn.Close()
		t.Fatal("expected connection to be rejected for empty origin header (security fix)")
	}
	// Connection should be rejected with bad handshake
	if !strings.Contains(err.Error(), "bad handshake") {
		t.Fatalf("expected bad handshake error, got: %v", err)
	}
}

func TestDeployHub_OriginValidation_StrictDefault(t *testing.T) {
	hub := NewDeployHub()
	// Default: empty allowedOrigins = strict (no external origins allowed)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-origin5")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	header := http.Header{}
	header.Set("Origin", "http://anything.example.com")
	_, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Fatal("expected connection to be rejected with empty allowedOrigins")
	}
}
