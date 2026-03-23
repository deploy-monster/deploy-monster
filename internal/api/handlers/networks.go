package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// NetworkHandler manages container network operations.
type NetworkHandler struct {
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewNetworkHandler(runtime core.ContainerRuntime, events *core.EventBus) *NetworkHandler {
	return &NetworkHandler{runtime: runtime, events: events}
}

// List handles GET /api/v1/networks
func (h *NetworkHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	// List monster-managed networks via container labels
	containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.enable": "true",
	})

	networks := make(map[string]bool)
	for _, c := range containers {
		if stack := c.Labels["monster.stack"]; stack != "" {
			networks["monster-"+stack+"-net"] = true
		}
	}
	networks["monster-network"] = true

	result := make([]string, 0, len(networks))
	for n := range networks {
		result = append(result, n)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

type connectNetworkRequest struct {
	ContainerID string `json:"container_id"`
	Network     string `json:"network"`
}

// Connect handles POST /api/v1/networks/connect
func (h *NetworkHandler) Connect(w http.ResponseWriter, r *http.Request) {
	var req connectNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Docker network connect would happen here
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "connected",
		"container":   req.ContainerID,
		"network":     req.Network,
	})
}
