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
