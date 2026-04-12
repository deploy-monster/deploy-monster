package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DiskUsageHandler shows container and volume disk usage.
type DiskUsageHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewDiskUsageHandler(store core.Store, runtime core.ContainerRuntime) *DiskUsageHandler {
	return &DiskUsageHandler{store: store, runtime: runtime}
}

// AppDisk handles GET /api/v1/apps/{id}/disk
func (h *DiskUsageHandler) AppDisk(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"app_id": appID, "container_size_mb": 0, "volume_size_mb": 0, "log_size_mb": 0,
		})
		return
	}

	containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":            appID,
		"containers":        len(containers),
		"container_size_mb": 0,
		"volume_size_mb":    0,
		"log_size_mb":       0,
		"total_mb":          0,
	})
}

// SystemDisk handles GET /api/v1/admin/disk
func (h *DiskUsageHandler) SystemDisk(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"containers_mb":  0,
		"images_mb":      0,
		"volumes_mb":     0,
		"build_cache_mb": 0,
		"total_mb":       0,
		"available_mb":   0,
	})
}
