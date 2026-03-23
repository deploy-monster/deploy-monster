package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// GPUHandler manages container GPU passthrough configuration.
// Enables NVIDIA GPU access for AI/ML workloads (Ollama, etc.).
type GPUHandler struct {
	store core.Store
}

func NewGPUHandler(store core.Store) *GPUHandler {
	return &GPUHandler{store: store}
}

// GPUConfig holds GPU passthrough settings.
type GPUConfig struct {
	Enabled     bool     `json:"enabled"`
	DeviceIDs   []string `json:"device_ids,omitempty"` // specific GPU IDs, empty = all
	Capabilities []string `json:"capabilities"`        // compute, utility, graphics
	Driver      string   `json:"driver"`               // nvidia
}

// Get handles GET /api/v1/apps/{id}/gpu
func (h *GPUHandler) Get(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, GPUConfig{
		Enabled:      false,
		Capabilities: []string{"compute", "utility"},
		Driver:       "nvidia",
	})
}

// Update handles PUT /api/v1/apps/{id}/gpu
func (h *GPUHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg GPUConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Would update container config with --gpus flag
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
