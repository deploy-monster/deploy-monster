package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// =============================================================================
// evictDead
// =============================================================================

func TestDeployHub_evictDead(t *testing.T) {
	hub := NewDeployHub()
	ready := make(chan struct{})

	// Server that registers two clients then closes one intentionally
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hub.Register("proj-evict", conn)
		close(ready)

		// Block until closed
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws1.Close()

	<-ready

	// Manually inject a dead client wrapper to evict
	hub.mu.RLock()
	var deadConn *websocket.Conn
	for c := range hub.clients["proj-evict"] {
		deadConn = c
		break
	}
	hub.mu.RUnlock()

	if deadConn == nil {
		t.Fatal("no connection found")
	}

	// Evict it directly
	hub.evictDead("proj-evict", []*clientConn{{conn: deadConn}})

	// Verify it's gone
	hub.mu.RLock()
	count := len(hub.clients["proj-evict"])
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients after evict, got %d", count)
	}
}

// =============================================================================
// Broadcast helpers
// =============================================================================

func TestBroadcastHelpers_NoPanic(t *testing.T) {
	// These should not panic even with no clients
	BroadcastValidating("proj-broadcast-1")
	BroadcastCompiling("proj-broadcast-2")
	BroadcastDeploying("proj-broadcast-3")
	BroadcastSuccess("proj-broadcast-4", "5s", []string{"c1"}, []string{"n1"}, []string{"v1"})
	BroadcastError("proj-broadcast-5", "fail", []string{"err1"})
}

func testBroadcastHelper(t *testing.T, broadcastFn func(*DeployHub), projectID, want string) {
	t.Helper()
	hub := NewDeployHub()
	ready := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hub.Register(projectID, conn)
		defer hub.Unregister(projectID, conn)

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

	oldHub := deployHub
	deployHub = hub
	defer func() { deployHub = oldHub }()

	broadcastFn(hub)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("expected %q in message, got %s", want, string(data))
	}
}

func TestBroadcastValidating_WithClient(t *testing.T) {
	testBroadcastHelper(t, func(hub *DeployHub) {
		BroadcastValidating("proj-broadcast")
	}, "proj-broadcast", "validating")
}

func TestBroadcastCompiling_WithClient(t *testing.T) {
	testBroadcastHelper(t, func(hub *DeployHub) {
		BroadcastCompiling("proj-broadcast")
	}, "proj-broadcast", "compiling")
}

func TestBroadcastDeploying_WithClient(t *testing.T) {
	testBroadcastHelper(t, func(hub *DeployHub) {
		BroadcastDeploying("proj-broadcast")
	}, "proj-broadcast", "deploying")
}

func TestBroadcastSuccess_WithClient(t *testing.T) {
	testBroadcastHelper(t, func(hub *DeployHub) {
		BroadcastSuccess("proj-broadcast", "10s", []string{"c1"}, nil, nil)
	}, "proj-broadcast", "Deployment completed successfully")
}

func TestBroadcastError_WithClient(t *testing.T) {
	testBroadcastHelper(t, func(hub *DeployHub) {
		BroadcastError("proj-broadcast", "boom", []string{"e1"})
	}, "proj-broadcast", "boom")
}

// =============================================================================
// Shutdown
// =============================================================================

func TestShutdown_Idempotent(t *testing.T) {
	// Use a fresh hub for this test
	oldHub := deployHub
	deployHub = NewDeployHub()
	defer func() { deployHub = oldHub }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Shutdown(ctx); err != nil {
		t.Errorf("first shutdown: %v", err)
	}

	// Second call should also succeed (idempotent)
	if err := Shutdown(ctx); err != nil {
		t.Errorf("second shutdown: %v", err)
	}
}

func TestDeployHub_Shutdown_WithClients(t *testing.T) {
	hub := NewDeployHub()
	ready := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hub.Register("proj-shutdown", conn)
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := hub.Shutdown(ctx); err != nil {
		t.Errorf("shutdown: %v", err)
	}

	// Hub should be closed
	if !hub.closed {
		t.Error("hub should be closed after shutdown")
	}
}
