package handlers

import (
	"net/http"
	stdpath "path"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// FileBrowserHandler provides read-only access to container filesystem.
type FileBrowserHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewFileBrowserHandler(store core.Store, runtime core.ContainerRuntime) *FileBrowserHandler {
	return &FileBrowserHandler{store: store, runtime: runtime}
}

// FileEntry represents a file or directory in a container.
type FileEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // file, directory
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

// isPathSafe checks if a path is safe from traversal attacks.
func isPathSafe(p string) bool {
	// Ensure path starts with /
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	// Block path traversal attempts
	if strings.Contains(p, "..") {
		return false
	}
	// Block null bytes
	if strings.Contains(p, "\x00") {
		return false
	}
	// Block absolute paths outside root (Windows drive letters)
	if len(p) >= 2 && p[1] == ':' && (p[0] >= 'A' && p[0] <= 'Z') {
		return false
	}
	// Use path.Clean (forward slashes) instead of filepath.Clean (OS-specific)
	cleaned := stdpath.Clean(p)
	return strings.HasPrefix(cleaned, "/")
}

// List handles GET /api/v1/apps/{id}/files?path=/
// Lists files in a container directory.
func (h *FileBrowserHandler) List(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	// Security: Validate path to prevent traversal attacks
	if !isPathSafe(path) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
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
