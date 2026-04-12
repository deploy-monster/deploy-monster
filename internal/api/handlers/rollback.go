package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/deploy"
)

// RollbackHandler manages deployment rollback operations.
type RollbackHandler struct {
	store  core.Store
	engine *deploy.RollbackEngine
}

func NewRollbackHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *RollbackHandler {
	return &RollbackHandler{
		store:  store,
		engine: deploy.NewRollbackEngine(store, runtime, events),
	}
}

type rollbackRequest struct {
	Version int `json:"version"`
}

// Rollback handles POST /api/v1/apps/{id}/rollback
func (h *RollbackHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "version must be positive")
		return
	}

	dep, err := h.engine.Rollback(r.Context(), app.ID, req.Version)
	if err != nil {
		internalErrorCtx(r.Context(), w, "rollback failed", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deployment":     dep,
		"rolled_back_to": req.Version,
	})
}

// ListVersions handles GET /api/v1/apps/{id}/versions
func (h *RollbackHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	versions, err := h.engine.ListVersions(r.Context(), app.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": versions})
}
