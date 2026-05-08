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

	var volumeCount int
	var volumeTotalMB int64
	var imageCount int
	var imageTotalMB int64
	runtimeAvailable := h.runtime != nil

	if runtimeAvailable {
		if volumes, err := h.runtime.VolumeList(r.Context()); err == nil {
			volumeCount = len(volumes)
			for _, v := range volumes {
				if v.Mountpoint == "" {
					continue
				}
				volumeTotalMB += dirSize(v.Mountpoint) / (1024 * 1024)
			}
		}
		if images, err := h.runtime.ImageList(r.Context()); err == nil {
			imageCount = len(images)
			for _, img := range images {
				imageTotalMB += img.Size / (1024 * 1024)
			}
		}
	}

	// Backup byte total is read from the local backup storage rather than
	// a denormalised cache so it can't drift after a manual file delete.
	var backupCount int
	var backupTotalMB int64
	if h.bolt != nil {
		var backupStats struct {
			Count   int   `json:"count"`
			TotalMB int64 `json:"total_mb"`
		}
		if h.bolt.Get("metrics_ring", "backup_stats:"+claims.TenantID, &backupStats) == nil {
			backupCount = backupStats.Count
			backupTotalMB = backupStats.TotalMB
		}
	}

	resp := map[string]any{
		"tenant_id": claims.TenantID,
		"volumes": map[string]any{
			"count":    volumeCount,
			"total_mb": volumeTotalMB,
		},
		"backups": map[string]any{
			"count":    backupCount,
			"total_mb": backupTotalMB,
		},
		"databases": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
		"images": map[string]any{
			"count":    imageCount,
			"total_mb": imageTotalMB,
		},
	}
	if !runtimeAvailable {
		resp["runtime"] = "unavailable"
	}
	writeJSON(w, http.StatusOK, resp)
}
