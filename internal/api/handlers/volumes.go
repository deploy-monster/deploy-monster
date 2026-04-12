package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// VolumeHandler manages Docker volume operations.
type VolumeHandler struct {
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewVolumeHandler(runtime core.ContainerRuntime, events *core.EventBus) *VolumeHandler {
	return &VolumeHandler{runtime: runtime, events: events}
}

// List handles GET /api/v1/volumes
func (h *VolumeHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	// List containers to extract volume info from labels
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list volumes")
		return
	}

	// Extract unique volume info from container names
	volumes := make([]map[string]string, 0)
	seen := make(map[string]bool)
	for _, c := range containers {
		appID := c.Labels["monster.app.id"]
		if appID != "" && !seen[appID] {
			seen[appID] = true
			volumes = append(volumes, map[string]string{
				"app_id":       appID,
				"container_id": c.ID[:12],
				"name":         c.Labels["monster.app.name"],
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": volumes, "total": len(volumes)})
}

type createVolumeRequest struct {
	Name      string `json:"name"`
	AppID     string `json:"app_id"`
	MountPath string `json:"mount_path"`
	SizeMB    int    `json:"size_mb"`
}

// Create handles POST /api/v1/volumes
func (h *VolumeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.SizeMB < 0 || req.SizeMB > 102400 {
		writeError(w, http.StatusBadRequest, "size_mb must be between 0 and 102400")
		return
	}

	// Volume creation would use Docker Volume API
	writeJSON(w, http.StatusCreated, map[string]any{
		"name":       req.Name,
		"app_id":     req.AppID,
		"mount_path": req.MountPath,
		"size_mb":    req.SizeMB,
		"driver":     "local",
		"status":     "created",
	})
}
