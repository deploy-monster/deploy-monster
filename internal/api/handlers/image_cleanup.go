package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ImageCleanupHandler manages Docker image pruning.
type ImageCleanupHandler struct {
	runtime core.ContainerRuntime
}

func NewImageCleanupHandler(runtime core.ContainerRuntime) *ImageCleanupHandler {
	return &ImageCleanupHandler{runtime: runtime}
}

// DanglingImages handles GET /api/v1/images/dangling
func (h *ImageCleanupHandler) DanglingImages(w http.ResponseWriter, r *http.Request) {
	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		internalError(w, "failed to list images", err)
		return
	}

	var danglingCount int
	var reclaimableMB int64
	for _, img := range images {
		if len(img.Tags) == 0 || (len(img.Tags) == 1 && img.Tags[0] == "<none>:<none>") {
			danglingCount++
			reclaimableMB += img.Size / (1024 * 1024)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"dangling_count": danglingCount,
		"reclaimable_mb": reclaimableMB,
	})
}

// Prune handles DELETE /api/v1/images/prune
// Removes unused and dangling images.
func (h *ImageCleanupHandler) Prune(w http.ResponseWriter, r *http.Request) {
	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		internalError(w, "failed to list images", err)
		return
	}

	var removed int
	var reclaimedMB int64
	for _, img := range images {
		if len(img.Tags) == 0 || (len(img.Tags) == 1 && img.Tags[0] == "<none>:<none>") {
			if err := h.runtime.ImageRemove(r.Context(), img.ID); err != nil {
				continue // skip images that can't be removed (in use)
			}
			removed++
			reclaimedMB += img.Size / (1024 * 1024)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reclaimed_mb":   reclaimedMB,
		"images_removed": removed,
		"status":         "pruned",
	})
}
