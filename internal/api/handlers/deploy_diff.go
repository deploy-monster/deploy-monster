package handlers

import (
	"net/http"
	"strconv"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployDiffHandler compares two deployment versions.
type DeployDiffHandler struct {
	store core.Store
}

func NewDeployDiffHandler(store core.Store) *DeployDiffHandler {
	return &DeployDiffHandler{store: store}
}

// Diff handles GET /api/v1/apps/{id}/deployments/diff?from=1&to=2
func (h *DeployDiffHandler) Diff(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	fromVer, _ := strconv.Atoi(r.URL.Query().Get("from"))
	toVer, _ := strconv.Atoi(r.URL.Query().Get("to"))

	if fromVer <= 0 || toVer <= 0 {
		writeError(w, http.StatusBadRequest, "from and to version numbers required")
		return
	}

	deployments, err := h.store.ListDeploymentsByApp(r.Context(), appID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var fromDep, toDep *core.Deployment
	for i := range deployments {
		if deployments[i].Version == fromVer {
			fromDep = &deployments[i]
		}
		if deployments[i].Version == toVer {
			toDep = &deployments[i]
		}
	}

	if fromDep == nil || toDep == nil {
		writeError(w, http.StatusNotFound, "one or both versions not found")
		return
	}

	diff := map[string]any{
		"app_id": appID,
		"from":   fromVer,
		"to":     toVer,
		"changes": map[string]any{
			"image": map[string]string{
				"from": fromDep.Image,
				"to":   toDep.Image,
			},
			"commit": map[string]string{
				"from": fromDep.CommitSHA,
				"to":   toDep.CommitSHA,
			},
			"strategy": map[string]string{
				"from": fromDep.Strategy,
				"to":   toDep.Strategy,
			},
			"triggered_by": map[string]string{
				"from": fromDep.TriggeredBy,
				"to":   toDep.TriggeredBy,
			},
		},
	}

	writeJSON(w, http.StatusOK, diff)
}
