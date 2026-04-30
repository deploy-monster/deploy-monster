package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/gorilla/websocket"
)

// DeployProgressMessage represents a deployment progress update
type DeployProgressMessage struct {
	Type      string `json:"type"` // "deploy_progress"
	ProjectID string `json:"projectId"`
	Stage     string `json:"stage"` // validating, compiling, building, deploying, success, error
	Message   string `json:"message"`
	Progress  int    `json:"progress"` // 0-100
	Timestamp int64  `json:"timestamp"`
}

// DeployCompleteMessage represents a completed deployment
type DeployCompleteMessage struct {
	Type       string   `json:"type"` // "deploy_complete"
	ProjectID  string   `json:"projectId"`
	Success    bool     `json:"success"`
	Message    string   `json:"message"`
	Duration   string   `json:"duration"`
	Containers []string `json:"containers,omitempty"`
	Networks   []string `json:"networks,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`
	Errors     []string `json:"errors,omitempty"`
	Timestamp  int64    `json:"timestamp"`
}

// clientConn wraps a gorilla/websocket connection with a write mutex.
//
// Tier 77: gorilla/websocket explicitly requires that concurrent writes
// to the same connection be serialized by the caller ("Connections
// support one concurrent reader and one concurrent writer" — godoc).
// Pre-Tier-77, multiple concurrent BroadcastProgress / BroadcastComplete
// calls on the same conn raced inside beginMessage → messageWriter and
// could interleave WebSocket frames on the wire, producing JSON-parse
// failures on the client side.
type clientConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// safeWrite serializes writes on the wrapped connection via writeMu and
// applies the standard write deadline. All writes that touch a
// registered conn (broadcasts and pings) go through this helper.
func (cc *clientConn) safeWrite(messageType int, data []byte) error {
	cc.writeMu.Lock()
	defer cc.writeMu.Unlock()
	_ = cc.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return cc.conn.WriteMessage(messageType, data)
}

// DeployHub manages WebSocket connections for deployment progress.
//
// Tier 77 hardening:
//
//   - sync.Once-guarded Shutdown with a closed flag and wg.Wait drain
//     of in-flight ServeWS handlers — pre-Tier-77 the hub had no
//     Shutdown method at all, so api.Module.Stop left handler
//     goroutines running after the HTTP server had gone down.
//   - per-conn writeMu prevents concurrent-write frame corruption.
//   - broadcasts snapshot the client list under RLock, release the
//     lock, then write — no network I/O is performed under h.mu, so
//     Register / Unregister are never starved by in-flight broadcasts.
//   - dead clients are evicted when a broadcast write fails, preventing
//     permanent accumulation of stale conns.
//   - Register and ServeWS refuse new clients after Shutdown.
type DeployHub struct {
	clients        map[string]map[*websocket.Conn]*clientConn // projectID -> connections
	mu             sync.RWMutex
	logger         *slog.Logger
	allowedOrigins string // comma-separated allowed origins, "*" for all

	wg       sync.WaitGroup // tracks in-flight ServeWS handlers
	stopOnce sync.Once
	closed   bool // guarded by mu
}

// NewDeployHub creates a new deployment hub
func NewDeployHub() *DeployHub {
	return &DeployHub{
		clients:        make(map[string]map[*websocket.Conn]*clientConn),
		logger:         slog.Default(),
		allowedOrigins: "", // strict by default
	}
}

// SetAllowedOrigins configures which origins may open WebSocket connections.
// Pass "*" to allow all, or a comma-separated list of origins.
func (h *DeployHub) SetAllowedOrigins(origins string) {
	h.allowedOrigins = origins
}

func (h *DeployHub) upgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if h.allowedOrigins == "*" {
				return true
			}
			origin := r.Header.Get("Origin")
			// SECURITY FIX: Empty origin header no longer allowed
			// This prevents cross-origin WebSocket connections from tools that don't send Origin headers
			if origin == "" {
				h.logger.Warn("WebSocket connection rejected: empty origin header")
				return false
			}
			for _, allowed := range strings.Split(h.allowedOrigins, ",") {
				if strings.TrimSpace(allowed) == origin {
					return true
				}
			}
			h.logger.Warn("WebSocket origin rejected", "origin", origin)
			return false
		},
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}
}

// enter reserves an in-flight handler slot under the same lock that
// guards the closed flag. Returns false if the hub has been shut down —
// the caller MUST bail out without opening any further resources.
// Every successful enter() must be paired with exactly one leave().
func (h *DeployHub) enter() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.wg.Add(1)
	return true
}

// leave releases a handler slot reserved by enter().
func (h *DeployHub) leave() {
	h.wg.Done()
}

// Register adds a client connection for a project. Returns the client
// wrapper, or nil if the hub has been shut down. A successful Register
// must be paired with a deferred Unregister.
func (h *DeployHub) Register(projectID string, conn *websocket.Conn) *clientConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	if h.clients[projectID] == nil {
		h.clients[projectID] = make(map[*websocket.Conn]*clientConn)
	}
	cc := &clientConn{conn: conn}
	h.clients[projectID][conn] = cc

	h.logger.Debug("WebSocket client registered",
		"project", projectID,
		"total", len(h.clients[projectID]))
	return cc
}

// Unregister removes a client connection. Idempotent: a double
// Unregister (e.g. from ServeWS's defer after Shutdown already evicted
// the conn, or after evictDead removed it) is a no-op.
func (h *DeployHub) Unregister(projectID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns, ok := h.clients[projectID]
	if !ok {
		return
	}
	if _, exists := conns[conn]; !exists {
		return
	}
	delete(conns, conn)
	if len(conns) == 0 {
		delete(h.clients, projectID)
	}
	h.logger.Debug("WebSocket client unregistered", "project", projectID)
}

// BroadcastProgress sends a progress update to all clients for a project.
//
// Tier 77: the snapshot-and-release pattern below means broadcasts do
// NOT hold h.mu while writing to the network. Per-conn writeMu (via
// safeWrite) serializes concurrent broadcasts on the same connection
// and prevents gorilla/websocket frame corruption.
func (h *DeployHub) BroadcastProgress(projectID, stage, message string, progress int) {
	msg := DeployProgressMessage{
		Type:      "deploy_progress",
		ProjectID: projectID,
		Stage:     stage,
		Message:   message,
		Progress:  progress,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal progress message", "error", err)
		return
	}

	snapshot := h.snapshotClients(projectID)
	if len(snapshot) == 0 {
		return
	}

	var dead []*clientConn
	for _, cc := range snapshot {
		if err := cc.safeWrite(websocket.TextMessage, data); err != nil {
			h.logger.Warn("Failed to send progress message",
				"project", projectID, "error", err)
			dead = append(dead, cc)
		}
	}

	if len(dead) > 0 {
		h.evictDead(projectID, dead)
	}
}

// BroadcastComplete sends a completion message to all clients for a project.
func (h *DeployHub) BroadcastComplete(projectID string, success bool, message string, duration string, containers, networks, volumes, errors []string) {
	msg := DeployCompleteMessage{
		Type:       "deploy_complete",
		ProjectID:  projectID,
		Success:    success,
		Message:    message,
		Duration:   duration,
		Containers: containers,
		Networks:   networks,
		Volumes:    volumes,
		Errors:     errors,
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal complete message", "error", err)
		return
	}

	snapshot := h.snapshotClients(projectID)
	if len(snapshot) == 0 {
		return
	}

	var dead []*clientConn
	for _, cc := range snapshot {
		if err := cc.safeWrite(websocket.TextMessage, data); err != nil {
			h.logger.Warn("Failed to send complete message",
				"project", projectID, "error", err)
			dead = append(dead, cc)
		}
	}

	if len(dead) > 0 {
		h.evictDead(projectID, dead)
	}
}

// snapshotClients returns a copy of the client wrappers registered for
// a project. Callers must not hold h.mu when calling this — it acquires
// h.mu.RLock internally and releases it before returning.
func (h *DeployHub) snapshotClients(projectID string) []*clientConn {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return nil
	}
	conns, ok := h.clients[projectID]
	if !ok || len(conns) == 0 {
		return nil
	}
	snapshot := make([]*clientConn, 0, len(conns))
	for _, cc := range conns {
		snapshot = append(snapshot, cc)
	}
	return snapshot
}

// evictDead removes a set of dead client wrappers from the hub and
// closes their underlying connections. Called after a broadcast write
// failure so dead clients don't accumulate forever. Idempotent against
// concurrent Unregister / Shutdown — clients that are no longer in the
// map are simply closed (Conn.Close is safe to call more than once).
func (h *DeployHub) evictDead(projectID string, dead []*clientConn) {
	h.mu.Lock()
	conns, ok := h.clients[projectID]
	var closedList []*clientConn
	if ok {
		for _, cc := range dead {
			if _, exists := conns[cc.conn]; exists {
				delete(conns, cc.conn)
				closedList = append(closedList, cc)
			}
		}
		if len(conns) == 0 {
			delete(h.clients, projectID)
		}
	}
	h.mu.Unlock()

	// Close the underlying sockets outside the hub lock. We close every
	// dead conn we were handed — even ones that were already removed by
	// a concurrent Unregister/Shutdown — because conn.Close is idempotent
	// and we want to guarantee no dangling FDs on the broadcast's side.
	for _, cc := range dead {
		_ = cc.conn.Close()
	}
	if len(closedList) > 0 {
		h.logger.Debug("evicted dead WebSocket clients",
			"project", projectID, "count", len(closedList))
	}
}

// Shutdown stops the hub and drains in-flight ServeWS handlers.
//
// Tier 77: idempotent (stopOnce-guarded). The first call flips the
// closed flag, snapshots the client map, closes every registered
// connection (which unblocks each read loop with an error), then waits
// on wg for all in-flight ServeWS handlers to return. Subsequent calls
// simply block on the same wg drain — they do not double-close
// connections. Respects ctx: if ctx expires before the drain completes
// Shutdown returns ctx.Err() without panicking.
func (h *DeployHub) Shutdown(ctx context.Context) error {
	h.stopOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		snapshot := h.clients
		h.clients = make(map[string]map[*websocket.Conn]*clientConn)
		h.mu.Unlock()

		evicted := 0
		for _, conns := range snapshot {
			for _, cc := range conns {
				_ = cc.conn.Close()
				evicted++
			}
		}
		h.logger.Info("deploy hub shutting down", "evicted_clients", evicted)
	})

	// Drain in-flight handlers. wg.Wait lives outside stopOnce so a
	// second Shutdown still blocks on the drain.
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

const (
	wsPingInterval = 30 * time.Second
	wsPongTimeout  = 40 * time.Second // must be > pingInterval
	wsWriteTimeout = 10 * time.Second
)

// ServeWS handles WebSocket connections for deployment progress.
//
// Tier 77 hardening:
//
//   - enter/leave bracket the handler so Shutdown's wg.Wait drains it.
//   - closed-flag short-circuit rejects new connections during shutdown
//     with 503 instead of upgrading and then aborting.
//   - the ping goroutine uses cc.safeWrite for per-conn write
//     serialization — without this, a broadcast racing with a ping on
//     the same conn could corrupt frames.
//   - the handler waits for the ping goroutine to exit before returning
//     via a local pingWg, so the ping goroutine is never orphaned.
func (h *DeployHub) ServeWS(w http.ResponseWriter, r *http.Request, projectID string) {
	if !h.enter() {
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	}
	defer h.leave()

	up := h.upgrader()
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer func() { _ = conn.Close() }()

	// Set read limit to prevent memory exhaustion from large messages
	conn.SetReadLimit(4096)

	cc := h.Register(projectID, conn)
	if cc == nil {
		// Hub was shut down between enter() and Register. Best-effort
		// polite close before we bail.
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseServiceRestart, "server shutting down"),
			time.Now().Add(time.Second),
		)
		h.logger.Info("rejecting ws connection — hub is shutting down", "project", projectID)
		return
	}
	defer h.Unregister(projectID, conn)

	// Configure dead-connection detection: if no pong arrives within
	// wsPongTimeout the read loop will error out and clean up.
	_ = conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
		return nil
	})

	// Keep the connection alive with ping messages.
	//
	// Defer ordering matters here. The defers below run LIFO on exit:
	//   1. ticker.Stop()      — stop the ticker (optimization)
	//   2. close(done)        — signal the ping goroutine to return
	//   3. pingWg.Wait()      — wait for the ping goroutine to actually exit
	//
	// pingWg.Wait MUST run AFTER close(done), otherwise the ping
	// goroutine is still blocked on <-done and we deadlock.
	var pingWg sync.WaitGroup
	pingWg.Add(1)
	defer pingWg.Wait()

	done := make(chan struct{})
	defer close(done)

	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	go func() {
		defer pingWg.Done()
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic in websocket ping loop", "error", rec)
			}
		}()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// safeWrite serializes with any concurrent broadcast on
				// the same cc — Tier 77's per-conn writeMu.
				if err := cc.safeWrite(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Per-connection frame rate limiter. A single chatty client can no
	// longer pump the hub with thousands of frames/sec — at 100/sec
	// sustained and 200 burst this accommodates every legitimate UI
	// but costs a flooder its connection almost immediately.
	limiter := middleware.NewWSFrameLimiter(middleware.WSFrameRatePerSec, middleware.WSFrameBurst)

	// Read messages (client may send commands). When the conn is closed
	// — either by the client, by the ping deadline, or by Shutdown
	// closing it from under us — ReadMessage returns an error and we
	// break out to clean up. A frame that exceeds the bucket triggers
	// a policy-violation close so the client notices the throttle.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if !limiter.Allow() {
			h.logger.Warn("ws frame rate limit exceeded",
				"project", projectID, "remote", r.RemoteAddr)
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(
					websocket.ClosePolicyViolation,
					"frame rate limit exceeded",
				),
				time.Now().Add(time.Second),
			)
			return
		}
	}
}

// Global deploy hub instance
var deployHub = NewDeployHub()

// GetDeployHub returns the global deploy hub
func GetDeployHub() *DeployHub {
	return deployHub
}

// Shutdown tears down the global deploy hub. Called from
// api.Module.Stop to drain WebSocket handlers before the HTTP server
// stops accepting connections. Idempotent — wired in api.Module.Stop
// via Tier 77.
func Shutdown(ctx context.Context) error {
	return deployHub.Shutdown(ctx)
}

// Helper functions for broadcasting

// BroadcastValidating broadcasts validating stage
func BroadcastValidating(projectID string) {
	GetDeployHub().BroadcastProgress(projectID, "validating", "Validating topology configuration", 10)
}

// BroadcastCompiling broadcasts compiling stage
func BroadcastCompiling(projectID string) {
	GetDeployHub().BroadcastProgress(projectID, "compiling", "Generating Docker Compose configuration", 30)
}

// BroadcastDeploying broadcasts deploying stage
func BroadcastDeploying(projectID string) {
	GetDeployHub().BroadcastProgress(projectID, "deploying", "Starting services with Docker Compose", 80)
}

// BroadcastSuccess broadcasts successful deployment
func BroadcastSuccess(projectID string, duration string, containers, networks, volumes []string) {
	GetDeployHub().BroadcastProgress(projectID, "success", "Deployment completed successfully", 100)
	GetDeployHub().BroadcastComplete(projectID, true, "Deployment completed successfully", duration, containers, networks, volumes, nil)
}

// BroadcastError broadcasts deployment error
func BroadcastError(projectID string, message string, errors []string) {
	GetDeployHub().BroadcastComplete(projectID, false, message, "", nil, nil, nil, errors)
}
