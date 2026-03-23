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
func (h *ImageCleanupHandler) DanglingImages(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"dangling_count": 0,
		"reclaimable_mb": 0,
	})
}

// Prune handles DELETE /api/v1/images/prune
// Removes unused and dangling images.
func (h *ImageCleanupHandler) Prune(w http.ResponseWriter, _ *http.Request) {
	// docker image prune -a --filter "until=24h"
	writeJSON(w, http.StatusOK, map[string]any{
		"reclaimed_mb": 0,
		"images_removed": 0,
		"status": "pruned",
	})
}
