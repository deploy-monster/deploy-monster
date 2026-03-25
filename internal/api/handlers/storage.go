package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// StorageHandler tracks disk and volume usage per tenant.
type StorageHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	bolt    core.BoltStorer
}

func NewStorageHandler(store core.Store, runtime core.ContainerRuntime, bolt core.BoltStorer) *StorageHandler {
	return &StorageHandler{store: store, runtime: runtime, bolt: bolt}
}

// Usage handles GET /api/v1/storage/usage
func (h *StorageHandler) Usage(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get volume info from runtime
	var volumeCount int
	var volumeTotalMB int64
	volumes, err := h.runtime.VolumeList(r.Context())
	if err == nil {
		volumeCount = len(volumes)
	}

	// Get image info from runtime
	var imageCount int
	var imageTotalMB int64
	images, err := h.runtime.ImageList(r.Context())
	if err == nil {
		imageCount = len(images)
		for _, img := range images {
			imageTotalMB += img.Size / (1024 * 1024)
		}
	}

	// Check backup count from bolt cache
	var backupStats struct {
		Count   int   `json:"count"`
		TotalMB int64 `json:"total_mb"`
	}
	_ = h.bolt.Get("metrics_ring", "backup_stats:"+claims.TenantID, &backupStats)

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": claims.TenantID,
		"volumes": map[string]any{
			"count":    volumeCount,
			"total_mb": volumeTotalMB,
		},
		"backups": map[string]any{
			"count":    backupStats.Count,
			"total_mb": backupStats.TotalMB,
		},
		"databases": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
		"images": map[string]any{
			"count":    imageCount,
			"total_mb": imageTotalMB,
		},
	})
}
