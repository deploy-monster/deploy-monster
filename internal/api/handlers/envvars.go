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
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	// Parse stored env vars (encrypted JSON)
	var envVars []envVarEntry
	if app.EnvVarsEnc != "" {
		if err := json.Unmarshal([]byte(app.EnvVarsEnc), &envVars); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse env vars")
			return
		}
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
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var req struct {
		Vars []envVarEntry `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate keys and sizes
	const maxKeyLen = 256
	const maxValueLen = 64 * 1024  // 64 KB per value
	const maxTotalLen = 512 * 1024 // 512 KB total payload
	const maxVars = 500
	if len(req.Vars) > maxVars {
		writeError(w, http.StatusBadRequest, "too many env vars (max 500)")
		return
	}
	var totalSize int
	for _, v := range req.Vars {
		if v.Key == "" {
			writeError(w, http.StatusBadRequest, "empty key not allowed")
			return
		}
		if len(v.Key) > maxKeyLen {
			writeError(w, http.StatusBadRequest, "env var key exceeds 256 characters")
			return
		}
		if len(v.Value) > maxValueLen {
			writeError(w, http.StatusBadRequest, "env var value exceeds 64KB limit")
			return
		}
		totalSize += len(v.Key) + len(v.Value)
	}
	if totalSize > maxTotalLen {
		writeError(w, http.StatusBadRequest, "total env vars payload exceeds 512KB limit")
		return
	}

	// Serialize and store
	data, err := json.Marshal(req.Vars)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to serialize env vars")
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
	// Mask ${SECRET:...} references by showing only the prefix so it's clear
	// a secret is referenced, without exposing the secret name.
	if strings.HasPrefix(value, "${SECRET:") {
		return "${SECRET:***}"
	}

	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}
