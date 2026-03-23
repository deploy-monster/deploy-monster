package handlers

import (
	"net/http"
	"time"
)

// AgentStatusHandler shows connected agent node statuses.
type AgentStatusHandler struct{}

func NewAgentStatusHandler() *AgentStatusHandler {
	return &AgentStatusHandler{}
}

// AgentNodeStatus represents a connected agent's status.
type AgentNodeStatus struct {
	ServerID    string    `json:"server_id"`
	Hostname    string    `json:"hostname"`
	IPAddress   string    `json:"ip_address"`
	Status      string    `json:"status"` // connected, disconnected, unhealthy
	Version     string    `json:"version"`
	Containers  int       `json:"containers"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryMB    int64     `json:"memory_mb"`
	LastSeen    time.Time `json:"last_seen"`
}

// List handles GET /api/v1/agents
func (h *AgentStatusHandler) List(w http.ResponseWriter, _ *http.Request) {
	// Would query NodeManager for connected agents
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  []any{},
		"total": 0,
		"local": AgentNodeStatus{
			ServerID: "local",
			Hostname: "localhost",
			Status:   "connected",
			LastSeen: time.Now(),
		},
	})
}

// GetAgent handles GET /api/v1/agents/{id}
func (h *AgentStatusHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	writeJSON(w, http.StatusOK, AgentNodeStatus{
		ServerID: serverID,
		Status:   "unknown",
		LastSeen: time.Now(),
	})
}
