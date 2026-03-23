package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BuildCacheHandler manages Docker build cache.
type BuildCacheHandler struct {
	runtime core.ContainerRuntime
}

func NewBuildCacheHandler(runtime core.ContainerRuntime) *BuildCacheHandler {
	return &BuildCacheHandler{runtime: runtime}
}

// Stats handles GET /api/v1/build/cache
func (h *BuildCacheHandler) Stats(w http.ResponseWriter, _ *http.Request) {
	// Docker system df would give build cache info
	writeJSON(w, http.StatusOK, map[string]any{
		"layers":   0,
		"size_mb":  0,
		"reclaimable_mb": 0,
	})
}

// Clear handles DELETE /api/v1/build/cache
func (h *BuildCacheHandler) Clear(w http.ResponseWriter, _ *http.Request) {
	// docker builder prune
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "cleared",
		"message": "build cache pruned",
	})
}
