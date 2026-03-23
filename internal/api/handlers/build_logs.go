package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BuildLogHandler serves build log retrieval and download.
type BuildLogHandler struct {
	store core.Store
}

func NewBuildLogHandler(store core.Store) *BuildLogHandler {
	return &BuildLogHandler{store: store}
}

// Get handles GET /api/v1/apps/{id}/builds/{version}/log
func (h *BuildLogHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	_ = r.PathValue("version")

	dep, err := h.store.GetLatestDeployment(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no deployment found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":  appID,
		"version": dep.Version,
		"log":     dep.BuildLog,
		"status":  dep.Status,
	})
}

// Download handles GET /api/v1/apps/{id}/builds/{version}/log/download
func (h *BuildLogHandler) Download(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	dep, err := h.store.GetLatestDeployment(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no deployment found")
		return
	}

	filename := fmt.Sprintf("%s-build-v%d-%s.log", appID[:8], dep.Version, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	if dep.BuildLog != "" {
		w.Write([]byte(dep.BuildLog))
	} else {
		w.Write([]byte("No build log available for this deployment.\n"))
	}
}
