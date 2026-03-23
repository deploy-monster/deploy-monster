package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/deploy"
)

// RollbackHandler manages deployment rollback operations.
type RollbackHandler struct {
	engine *deploy.RollbackEngine
}

func NewRollbackHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *RollbackHandler {
	return &RollbackHandler{
		engine: deploy.NewRollbackEngine(store, runtime, events),
	}
}

type rollbackRequest struct {
	Version int `json:"version"`
}

// Rollback handles POST /api/v1/apps/{id}/rollback
func (h *RollbackHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "version must be positive")
		return
	}

	dep, err := h.engine.Rollback(r.Context(), appID, req.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rollback failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deployment": dep,
		"rolled_back_to": req.Version,
	})
}

// ListVersions handles GET /api/v1/apps/{id}/versions
func (h *RollbackHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	versions, err := h.engine.ListVersions(r.Context(), appID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": versions})
}
