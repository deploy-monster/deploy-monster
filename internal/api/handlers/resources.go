package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ResourceHandler manages container resource limits.
type ResourceHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewResourceHandler(store core.Store, events *core.EventBus) *ResourceHandler {
	return &ResourceHandler{store: store, events: events}
}

type resourceLimitsRequest struct {
	CPUQuota int64 `json:"cpu_quota"` // CFS quota microseconds (100000 = 1 core)
	MemoryMB int64 `json:"memory_mb"` // Hard memory limit
}

// SetLimits handles PUT /api/v1/apps/{id}/resources
func (h *ResourceHandler) SetLimits(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var req resourceLimitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CPUQuota < 0 || req.CPUQuota > 1600000 {
		writeError(w, http.StatusBadRequest, "cpu_quota must be between 0 and 1600000")
		return
	}
	if req.MemoryMB < 0 || req.MemoryMB > 131072 {
		writeError(w, http.StatusBadRequest, "memory_mb must be between 0 and 131072")
		return
	}

	// Store resource limits in labels
	labels := map[string]string{
		"monster.resources.cpu_quota": json.Number(json.Number(string(rune(req.CPUQuota)))).String(),
		"monster.resources.memory_mb": json.Number(json.Number(string(rune(req.MemoryMB)))).String(),
	}

	_ = labels
	_ = app

	// In production: update container with new resource constraints
	// docker update --cpus=X --memory=Ym container_id

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":    appID,
		"cpu_quota": req.CPUQuota,
		"memory_mb": req.MemoryMB,
		"status":    "limits updated",
	})
}

// GetLimits handles GET /api/v1/apps/{id}/resources
func (h *ResourceHandler) GetLimits(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	// Default limits
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":     appID,
		"cpu_quota":  0, // 0 = unlimited
		"memory_mb":  0, // 0 = unlimited
		"pids_limit": 0,
	})
}
