package ws

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// DeployHub manages WebSocket connections for deployment progress
type DeployHub struct {
	clients map[string]map[*websocket.Conn]bool // projectID -> connections
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewDeployHub creates a new deployment hub
func NewDeployHub() *DeployHub {
	return &DeployHub{
		clients: make(map[string]map[*websocket.Conn]bool),
		logger:  slog.Default(),
	}
}

// Register adds a client connection for a project
func (h *DeployHub) Register(projectID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[projectID] == nil {
		h.clients[projectID] = make(map[*websocket.Conn]bool)
	}
	h.clients[projectID][conn] = true

	h.logger.Debug("WebSocket client registered", "project", projectID, "total", len(h.clients[projectID]))
}

// Unregister removes a client connection
func (h *DeployHub) Unregister(projectID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns, ok := h.clients[projectID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, projectID)
		}
	}

	h.logger.Debug("WebSocket client unregistered", "project", projectID)
}

// BroadcastProgress sends a progress update to all clients for a project
func (h *DeployHub) BroadcastProgress(projectID, stage, message string, progress int) {
	h.mu.RLock()
	defer h.mu.RUnlock()

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

	if conns, ok := h.clients[projectID]; ok {
		for conn := range conns {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				h.logger.Warn("Failed to send progress message", "error", err)
			}
		}
	}
}

// BroadcastComplete sends a completion message to all clients
func (h *DeployHub) BroadcastComplete(projectID string, success bool, message string, duration string, containers, networks, volumes, errors []string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

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

	if conns, ok := h.clients[projectID]; ok {
		for conn := range conns {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				h.logger.Warn("Failed to send complete message", "error", err)
			}
		}
	}
}

// ServeWS handles WebSocket connections for deployment progress
func (h *DeployHub) ServeWS(w http.ResponseWriter, r *http.Request, projectID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	h.Register(projectID, conn)
	defer h.Unregister(projectID, conn)

	// Keep connection alive with ping messages.
	// done channel ensures the ping goroutine exits when the read loop ends.
	done := make(chan struct{})
	defer close(done)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read messages (client may send commands)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// Global deploy hub instance
var deployHub = NewDeployHub()

// GetDeployHub returns the global deploy hub
func GetDeployHub() *DeployHub {
	return deployHub
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

// BroadcastBuilding broadcasts building stage
func BroadcastBuilding(projectID string, service string) {
	GetDeployHub().BroadcastProgress(projectID, "building", fmt.Sprintf("Building image for %s", service), 50)
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
