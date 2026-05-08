package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DiskUsageHandler shows container and volume disk usage.
type DiskUsageHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	reader  core.SysMetricsReader
}

func NewDiskUsageHandler(store core.Store, runtime core.ContainerRuntime) *DiskUsageHandler {
	return &DiskUsageHandler{store: store, runtime: runtime, reader: core.NewSysMetricsReader()}
}

// AppDisk handles GET /api/v1/apps/{id}/disk.
// Reports the count of containers attached to the app and the cumulative
// size of the images those containers reference. We don't include the
// container's writable layer because the Docker SDK only exposes that via
// ContainerInspect with `Size: true`, which isn't on our runtime
// interface; surfacing 0 there used to look like real "no usage", so
// instead we leave that field omitted with `image_size_bytes` covering
// the meaningful component.
func (h *DiskUsageHandler) AppDisk(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"app_id":           appID,
			"containers":       0,
			"image_size_bytes": 0,
			"runtime":          "unavailable",
		})
		return
	}

	containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})

	imagesByID := map[string]bool{}
	for _, c := range containers {
		imagesByID[c.Image] = true
	}

	var imageBytes int64
	if len(imagesByID) > 0 {
		images, err := h.runtime.ImageList(r.Context())
		if err == nil {
			for _, img := range images {
				if imagesByID[img.ID] {
					imageBytes += img.Size
					continue
				}
				for _, tag := range img.Tags {
					if imagesByID[tag] {
						imageBytes += img.Size
						break
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":           appID,
		"containers":       len(containers),
		"image_size_bytes": imageBytes,
	})
}

// SystemDisk handles GET /api/v1/admin/disk.
// Combines the host filesystem snapshot from SysMetrics with Docker-side
// totals for images and (best-effort) volumes. Build cache is reported
// separately via /api/v1/build/cache and so isn't double-counted here.
func (h *DiskUsageHandler) SystemDisk(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"containers_bytes": 0,
		"images_bytes":     0,
		"volumes_bytes":    0,
		"total_bytes":      0,
		"available_bytes":  0,
		"used_bytes":       0,
	}

	if h.reader != nil {
		if m, err := h.reader.Read(); err == nil {
			resp["total_bytes"] = m.DiskTotalMB * 1024 * 1024
			resp["used_bytes"] = m.DiskUsedMB * 1024 * 1024
			resp["available_bytes"] = (m.DiskTotalMB - m.DiskUsedMB) * 1024 * 1024
		}
	}

	if h.runtime != nil {
		if images, err := h.runtime.ImageList(r.Context()); err == nil {
			var total int64
			for _, img := range images {
				total += img.Size
			}
			resp["images_bytes"] = total
			resp["images_count"] = len(images)
		}
		if volumes, err := h.runtime.VolumeList(r.Context()); err == nil {
			var total int64
			for _, v := range volumes {
				if v.Mountpoint == "" {
					continue
				}
				total += dirSize(v.Mountpoint)
			}
			resp["volumes_bytes"] = total
			resp["volumes_count"] = len(volumes)
		}
	} else {
		resp["runtime"] = "unavailable"
	}

	writeJSON(w, http.StatusOK, resp)
}

// dirSize sums the size of every regular file under root. Best-effort:
// permission errors silently return what was reachable. Volume mountpoints
// owned by root are common — when the platform process can't read them,
// the volumes_bytes total reflects only what's visible.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
				return nil
			}
			return nil
		}
		if info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
