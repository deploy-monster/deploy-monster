package handlers

import (
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ImageTagHandler lists available tags for Docker images.
type ImageTagHandler struct {
	runtime core.ContainerRuntime
}

func NewImageTagHandler(runtime core.ContainerRuntime) *ImageTagHandler {
	return &ImageTagHandler{runtime: runtime}
}

// TagInfo represents a Docker image tag.
type TagInfo struct {
	Name        string `json:"name"`
	Digest      string `json:"digest,omitempty"`
	Size        int64  `json:"size,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// List handles GET /api/v1/images/tags?image=nginx
// Lists tags for images available in the local Docker runtime.
func (h *ImageTagHandler) List(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("image")
	if image == "" {
		writeError(w, http.StatusBadRequest, "image query param required")
		return
	}

	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list images: "+err.Error())
		return
	}

	var tags []TagInfo
	for _, img := range images {
		for _, tag := range img.Tags {
			// Match by image name prefix (e.g., "nginx" matches "nginx:latest", "nginx:1.25")
			parts := strings.SplitN(tag, ":", 2)
			if len(parts) == 2 && (parts[0] == image || strings.HasSuffix(parts[0], "/"+image)) {
				tags = append(tags, TagInfo{
					Name:   parts[1],
					Digest: img.ID,
					Size:   img.Size,
				})
			}
		}
	}

	if tags == nil {
		tags = []TagInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"image": image,
		"tags":  tags,
		"total": len(tags),
	})
}
