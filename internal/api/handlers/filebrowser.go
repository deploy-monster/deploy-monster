package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// FileBrowserHandler provides read-only access to container filesystem.
type FileBrowserHandler struct {
	runtime core.ContainerRuntime
}

func NewFileBrowserHandler(runtime core.ContainerRuntime) *FileBrowserHandler {
	return &FileBrowserHandler{runtime: runtime}
}

// FileEntry represents a file or directory in a container.
type FileEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // file, directory
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

// List handles GET /api/v1/apps/{id}/files?path=/
// Lists files in a container directory.
func (h *FileBrowserHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no container found")
		return
	}

	// Would use docker exec "ls -la" or docker cp to read files
	// For now, return structural response
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"container_id": containers[0].ID[:12],
		"path":         path,
		"files":        []FileEntry{},
	})
}
