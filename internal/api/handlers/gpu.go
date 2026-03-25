package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// GPUHandler manages container GPU passthrough configuration.
// Enables NVIDIA GPU access for AI/ML workloads (Ollama, etc.).
type GPUHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	bolt    core.BoltStorer
}

func NewGPUHandler(store core.Store, runtime core.ContainerRuntime, bolt core.BoltStorer) *GPUHandler {
	return &GPUHandler{store: store, runtime: runtime, bolt: bolt}
}

// GPUConfig holds GPU passthrough settings.
type GPUConfig struct {
	Enabled      bool     `json:"enabled"`
	DeviceIDs    []string `json:"device_ids,omitempty"` // specific GPU IDs, empty = all
	Capabilities []string `json:"capabilities"`         // compute, utility, graphics
	Driver       string   `json:"driver"`               // nvidia
}

// gpuDetection holds detected GPU info from the host.
type gpuDetection struct {
	Available bool     `json:"available"`
	Devices   []string `json:"devices,omitempty"`
	Driver    string   `json:"driver,omitempty"`
}

// Get handles GET /api/v1/apps/{id}/gpu
func (h *GPUHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	// Try to load stored GPU config for this app
	var cfg GPUConfig
	if err := h.bolt.Get("gpu_config", appID, &cfg); err != nil {
		cfg = GPUConfig{
			Enabled:      false,
			Capabilities: []string{"compute", "utility"},
			Driver:       "nvidia",
		}
	}

	// Detect GPU availability on the host
	detection := h.detectGPU(r)

	writeJSON(w, http.StatusOK, map[string]any{
		"config":    cfg,
		"detection": detection,
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

	if len(cfg.Capabilities) == 0 {
		cfg.Capabilities = []string{"compute", "utility"}
	}
	if cfg.Driver == "" {
		cfg.Driver = "nvidia"
	}

	if err := h.bolt.Set("gpu_config", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save GPU config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}

// detectGPU checks for NVIDIA GPU devices on the host via the container runtime.
func (h *GPUHandler) detectGPU(r *http.Request) gpuDetection {
	if h.runtime == nil {
		return gpuDetection{Available: false}
	}

	// Try to list containers that have GPU labels — a heuristic approach.
	// In production, this would run nvidia-smi or check /dev/nvidia* devices.
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"com.docker.compose.service": "nvidia"})
	if err == nil && len(containers) > 0 {
		return gpuDetection{
			Available: true,
			Driver:    "nvidia",
		}
	}

	// Check for nvidia images as another heuristic
	images, err := h.runtime.ImageList(r.Context())
	if err == nil {
		for _, img := range images {
			for _, tag := range img.Tags {
				if strings.Contains(tag, "nvidia") || strings.Contains(tag, "cuda") {
					return gpuDetection{
						Available: true,
						Driver:    "nvidia",
					}
				}
			}
		}
	}

	return gpuDetection{Available: false}
}
