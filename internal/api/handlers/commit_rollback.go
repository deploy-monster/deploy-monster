package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CommitRollbackHandler handles rollback to a specific git commit.
type CommitRollbackHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewCommitRollbackHandler(store core.Store, events *core.EventBus) *CommitRollbackHandler {
	return &CommitRollbackHandler{store: store, events: events}
}

type commitRollbackRequest struct {
	CommitSHA string `json:"commit_sha"`
}

// RollbackToCommit handles POST /api/v1/apps/{id}/rollback-to-commit
// Finds the deployment that matches the commit and redeploys it.
func (h *CommitRollbackHandler) RollbackToCommit(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req commitRollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CommitSHA == "" {
		writeError(w, http.StatusBadRequest, "commit_sha required")
		return
	}

	// Find deployment with matching commit
	deployments, err := h.store.ListDeploymentsByApp(r.Context(), appID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var target *core.Deployment
	for i := range deployments {
		if deployments[i].CommitSHA == req.CommitSHA {
			target = &deployments[i]
			break
		}
		// Partial match (first 7+ chars)
		if len(req.CommitSHA) >= 7 && len(deployments[i].CommitSHA) >= len(req.CommitSHA) &&
			deployments[i].CommitSHA[:len(req.CommitSHA)] == req.CommitSHA {
			target = &deployments[i]
			break
		}
	}

	if target == nil {
		writeError(w, http.StatusNotFound, "no deployment found for commit "+req.CommitSHA)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":    appID,
		"commit":    target.CommitSHA,
		"version":   target.Version,
		"image":     target.Image,
		"message":   "use POST /api/v1/apps/{id}/rollback with this version number",
		"rollback_version": target.Version,
	})
}
