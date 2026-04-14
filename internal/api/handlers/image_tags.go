package handlers

import (
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ImageTagHandler lists available tags for Docker images.
type ImageTagHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewImageTagHandler(store core.Store, runtime core.ContainerRuntime) *ImageTagHandler {
	return &ImageTagHandler{store: store, runtime: runtime}
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
// SECURITY FIX (AUTHZ-007): Added authentication and tenant isolation.
func (h *ImageTagHandler) List(w http.ResponseWriter, r *http.Request) {
	// Verify authentication
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	image := r.URL.Query().Get("image")
	if image == "" {
		writeError(w, http.StatusBadRequest, "image query param required")
		return
	}

	// SECURITY FIX (AUTHZ-007): Verify the image is used by an app in this tenant
	// First, get all apps for this tenant
	apps, _, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 1000, 0)
	if err != nil {
		internalError(w, "failed to list apps", err)
		return
	}

	// Build a set of allowed image names from the tenant's deployments
	allowedImages := make(map[string]bool)
	for _, app := range apps {
		// Get the latest deployment for this app
		deploy, err := h.store.GetLatestDeployment(r.Context(), app.ID)
		if err == nil && deploy != nil && deploy.Image != "" {
			// Extract base image name without tag
			parts := strings.SplitN(deploy.Image, ":", 2)
			allowedImages[parts[0]] = true
		}
	}

	// Check if the requested image is in the allowed set
	if !allowedImages[image] {
		writeError(w, http.StatusForbidden, "access denied to this image")
		return
	}

	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		internalError(w, "failed to list images", err)
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
