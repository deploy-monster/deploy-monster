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

// Update handles PUT /api/v1/apps/{id}/restart-policy.
// The actual Docker `--restart` flag isn't mutable through the runtime
// interface (would require an UpdateContainer hook on
// core.ContainerRuntime — not currently exposed). Until that hook
// lands the policy is recorded on the application's labels JSON so the
// next deploy applies it. The response status names that explicitly so
// the operator isn't misled into thinking a live container changed.
func (h *RestartPolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

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

	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	labels := map[string]string{}
	if app.LabelsJSON != "" {
		_ = json.Unmarshal([]byte(app.LabelsJSON), &labels)
	}
	labels["restart_policy"] = req.Policy
	encoded, err := json.Marshal(labels)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode labels")
		return
	}
	app.LabelsJSON = string(encoded)
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist policy")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"app_id": appID,
		"policy": req.Policy,
		"status": "applied_at_next_deploy",
		"note":   "policy persisted on application; live container restart policy isn't mutable without a redeploy",
	})
}
