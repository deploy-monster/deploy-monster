package handlers

import (
	"encoding/json"
	"net/http"

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
// Returns env vars with raw values. The endpoint is JWT-protected and
// scoped to tenant members with read access; an additional masking layer
// would break round-trip PUT/GET CRUD for non-secret values.
// Secret references (${SECRET:name}) point to the encrypted vault, so the
// stored value is already a non-sensitive pointer.
func (h *EnvVarHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	envVars := make([]envVarEntry, 0)
	if app.EnvVarsEnc != "" {
		if err := json.Unmarshal([]byte(app.EnvVarsEnc), &envVars); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse env vars")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": envVars})
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
	if !decodeJSONInto(w, r, &req) {
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
