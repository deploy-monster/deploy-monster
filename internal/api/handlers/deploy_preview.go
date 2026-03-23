package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployPreviewHandler provides deployment dry-run / preview.
type DeployPreviewHandler struct {
	store core.Store
}

func NewDeployPreviewHandler(store core.Store) *DeployPreviewHandler {
	return &DeployPreviewHandler{store: store}
}

// Preview handles POST /api/v1/apps/{id}/deploy/preview
// Shows what would happen without actually deploying.
func (h *DeployPreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	currentDep, _ := h.store.GetLatestDeployment(r.Context(), appID)
	nextVersion, _ := h.store.GetNextDeployVersion(r.Context(), appID)

	// Detect what would be built
	var detectedType string
	if app.SourceType == "git" {
		detectedType = "would clone and auto-detect"
	} else if app.SourceType == "image" {
		detectedType = "image pull: " + app.SourceURL
	} else {
		detectedType = app.SourceType
	}

	// Check Dockerfile template availability
	var dockerfile string
	if app.SourceType == "git" {
		dockerfile = "auto-generated based on project type"
	} else if app.Dockerfile != "" {
		dockerfile = "custom: " + app.Dockerfile
	}

	preview := map[string]any{
		"app_id":       appID,
		"app_name":     app.Name,
		"source_type":  app.SourceType,
		"source_url":   app.SourceURL,
		"branch":       app.Branch,
		"current_version": 0,
		"next_version":   nextVersion,
		"strategy":      "recreate",
		"detected_type": detectedType,
		"dockerfile":   dockerfile,
		"supported_types": []string{
			string(build.TypeNodeJS), string(build.TypeNextJS), string(build.TypeGo),
			string(build.TypePython), string(build.TypeRust), string(build.TypePHP),
			string(build.TypeJava), string(build.TypeDotNet), string(build.TypeRuby),
			string(build.TypeVite), string(build.TypeNuxt), string(build.TypeStatic),
		},
		"dry_run": true,
	}

	if currentDep != nil {
		preview["current_version"] = currentDep.Version
		preview["current_image"] = currentDep.Image
	}

	writeJSON(w, http.StatusOK, preview)
}
