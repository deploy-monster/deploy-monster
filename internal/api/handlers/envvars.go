package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// EnvVarHandler manages application environment variables.
type EnvVarHandler struct {
	store core.Store
}

func NewEnvVarHandler(store core.Store) *EnvVarHandler {
	return &EnvVarHandler{store: store}
}

type envVarEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Get handles GET /api/v1/apps/{id}/env
// Returns env vars with secret values masked.
func (h *EnvVarHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	// Parse stored env vars (encrypted JSON)
	var envVars []envVarEntry
	if app.EnvVarsEnc != "" {
		json.Unmarshal([]byte(app.EnvVarsEnc), &envVars)
	}

	// Mask secret values
	masked := make([]envVarEntry, len(envVars))
	for i, ev := range envVars {
		masked[i] = envVarEntry{Key: ev.Key, Value: maskValue(ev.Value)}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": masked})
}

// Update handles PUT /api/v1/apps/{id}/env
func (h *EnvVarHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req struct {
		Vars []envVarEntry `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate keys
	for _, v := range req.Vars {
		if v.Key == "" {
			writeError(w, http.StatusBadRequest, "empty key not allowed")
			return
		}
	}

	// Serialize and store
	data, _ := json.Marshal(req.Vars)

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	app.EnvVarsEnc = string(data)

	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update env vars")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// maskValue hides the value, showing only first/last chars for non-secret references.
func maskValue(value string) string {
	// Don't mask ${SECRET:...} references — show them as-is
	if strings.HasPrefix(value, "${SECRET:") {
		return value
	}

	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}
