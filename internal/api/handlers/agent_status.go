package handlers

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AgentStatusHandler shows connected agent node statuses.
type AgentStatusHandler struct {
	core *core.Core
}

func NewAgentStatusHandler(c *core.Core) *AgentStatusHandler {
	return &AgentStatusHandler{core: c}
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
func (h *AgentStatusHandler) List(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()

	// Collect module health statuses
	health := h.core.Registry.HealthAll()
	moduleCount := len(h.core.Registry.All())
	healthyCount := 0
	for _, hs := range health {
		if hs == core.HealthOK {
			healthyCount++
		}
	}

	// Count containers via runtime
	var containerCount int
	if h.core.Services.Container != nil {
		containers, err := h.core.Services.Container.ListByLabels(r.Context(), nil)
		if err == nil {
			containerCount = len(containers)
		}
	}

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	localStatus := "connected"
	for _, hs := range health {
		if hs == core.HealthDown {
			localStatus = "unhealthy"
			break
		}
		if hs == core.HealthDegraded {
			localStatus = "degraded"
		}
	}

	local := AgentNodeStatus{
		ServerID:   "local",
		Hostname:   hostname,
		Status:     localStatus,
		Version:    h.core.Build.Version,
		Containers: containerCount,
		MemoryMB:   int64(memStats.Alloc / (1024 * 1024)),
		LastSeen:   time.Now(),
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":    []any{},
		"total":   0,
		"local":   local,
		"modules": moduleCount,
		"healthy": healthyCount,
	})
}

// GetAgent handles GET /api/v1/agents/{id}
func (h *AgentStatusHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")

	if serverID == "local" {
		// Redirect to the local agent info
		h.List(w, r)
		return
	}

	// Remote agents would be looked up from the node registry
	writeJSON(w, http.StatusOK, AgentNodeStatus{
		ServerID: serverID,
		Status:   "unknown",
		Version:  h.core.Build.Version,
		LastSeen: time.Now(),
	})
}
