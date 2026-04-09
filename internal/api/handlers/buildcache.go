package handlers

import (
	"log/slog"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BuildCacheHandler manages Docker build cache.
type BuildCacheHandler struct {
	runtime core.ContainerRuntime
	bolt    core.BoltStorer
}

func NewBuildCacheHandler(runtime core.ContainerRuntime, bolt core.BoltStorer) *BuildCacheHandler {
	return &BuildCacheHandler{runtime: runtime, bolt: bolt}
}

// buildCacheStats is the persisted build cache statistics.
type buildCacheStats struct {
	TotalBuilds   int   `json:"total_builds"`
	CacheHits     int   `json:"cache_hits"`
	CacheMisses   int   `json:"cache_misses"`
	TotalSavedSec int64 `json:"total_saved_sec"`
}

// Stats handles GET /api/v1/build/cache
func (h *BuildCacheHandler) Stats(w http.ResponseWriter, r *http.Request) {
	var layerCount int
	var totalSizeMB int64

	// Get image layer info from runtime as a proxy for build cache
	if h.runtime != nil {
		images, err := h.runtime.ImageList(r.Context())
		if err != nil {
			internalError(w, "failed to query runtime", err)
			return
		}
		layerCount = len(images)
		for _, img := range images {
			totalSizeMB += img.Size / (1024 * 1024)
		}
	}

	// Load persisted build cache stats
	var stats buildCacheStats
	_ = h.bolt.Get("buildcache", "stats", &stats)

	writeJSON(w, http.StatusOK, map[string]any{
		"layers":          layerCount,
		"size_mb":         totalSizeMB,
		"reclaimable_mb":  totalSizeMB / 4, // estimate ~25% reclaimable
		"total_builds":    stats.TotalBuilds,
		"cache_hits":      stats.CacheHits,
		"cache_misses":    stats.CacheMisses,
		"total_saved_sec": stats.TotalSavedSec,
	})
}

// Clear handles DELETE /api/v1/build/cache
func (h *BuildCacheHandler) Clear(w http.ResponseWriter, r *http.Request) {
	var removed int
	var reclaimedMB int64

	if h.runtime != nil {
		// Remove dangling images (build cache layers)
		images, err := h.runtime.ImageList(r.Context())
		if err != nil {
			internalError(w, "failed to list images", err)
			return
		}

		for _, img := range images {
			if len(img.Tags) == 0 || (len(img.Tags) == 1 && img.Tags[0] == "<none>:<none>") {
				if err := h.runtime.ImageRemove(r.Context(), img.ID); err != nil {
					continue
				}
				removed++
				reclaimedMB += img.Size / (1024 * 1024)
			}
		}
	}

	// Reset persisted stats
	if err := h.bolt.Set("buildcache", "stats", buildCacheStats{}, 0); err != nil {
		slog.Error("failed to reset build cache stats", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "cleared",
		"images_removed": removed,
		"reclaimed_mb":   reclaimedMB,
	})
}
