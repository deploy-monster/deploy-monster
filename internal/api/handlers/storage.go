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
}

func NewStorageHandler(store core.Store, runtime core.ContainerRuntime) *StorageHandler {
	return &StorageHandler{store: store, runtime: runtime}
}

// Usage handles GET /api/v1/storage/usage
func (h *StorageHandler) Usage(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// In production would calculate:
	// - Volume sizes from Docker volumes API
	// - Backup sizes from backup storage
	// - Database sizes from container stats
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": claims.TenantID,
		"volumes": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
		"backups": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
		"databases": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
		"images": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
	})
}
