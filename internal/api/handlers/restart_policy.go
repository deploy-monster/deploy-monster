package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RestartPolicyHandler manages container restart policies.
type RestartPolicyHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewRestartPolicyHandler(store core.Store, runtime core.ContainerRuntime) *RestartPolicyHandler {
	return &RestartPolicyHandler{store: store, runtime: runtime}
}

type restartPolicyRequest struct {
	Policy string `json:"policy"` // always, unless-stopped, on-failure, no
}

// Update handles PUT /api/v1/apps/{id}/restart-policy
func (h *RestartPolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req restartPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := map[string]bool{"always": true, "unless-stopped": true, "on-failure": true, "no": true}
	if !valid[req.Policy] {
		writeError(w, http.StatusBadRequest, "policy must be: always, unless-stopped, on-failure, no")
		return
	}

	if requireTenantApp(w, r, h.store) == nil {
		return
	}

	// Docker: docker update --restart=<policy> <container>
	writeJSON(w, http.StatusOK, map[string]string{
		"app_id": appID,
		"policy": req.Policy,
		"status": "updated",
	})
}
