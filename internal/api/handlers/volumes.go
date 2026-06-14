package handlers

import (
	"context"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// VolumeHandler manages Docker volume operations.
type VolumeHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
}

func NewVolumeHandler(runtime core.ContainerRuntime, store core.Store, events *core.EventBus) *VolumeHandler {
	return &VolumeHandler{runtime: runtime, store: store, events: events}
}

// List handles GET /api/v1/volumes
func (h *VolumeHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
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
		if appID != "" && !seen[appID] && h.appVisibleToTenant(r.Context(), appID, claims.TenantID, c.Labels) {
			seen[appID] = true
			volumes = append(volumes, map[string]string{
				"app_id":       appID,
				"container_id": shortResourceID(c.ID),
				"name":         c.Labels["monster.app.name"],
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": volumes, "total": len(volumes)})
}

func (h *VolumeHandler) appVisibleToTenant(ctx context.Context, appID, tenantID string, labels map[string]string) bool {
	if labelTenant := labels["monster.tenant"]; labelTenant != "" {
		return labelTenant == tenantID
	}
	if h.store == nil {
		return false
	}
	app, err := h.store.GetApp(ctx, appID)
	return err == nil && app != nil && app.TenantID == tenantID
}

type createVolumeRequest struct {
	Name      string `json:"name"`
	AppID     string `json:"app_id"`
	MountPath string `json:"mount_path"`
	SizeMB    int    `json:"size_mb"`
}

// Create handles POST /api/v1/volumes
func (h *VolumeHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createVolumeRequest
	if !decodeJSONInto(w, r, &req) {
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
	if req.AppID != "" && !h.appVisibleToTenant(r.Context(), req.AppID, claims.TenantID, nil) {
		writeError(w, http.StatusNotFound, "app not found")
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
