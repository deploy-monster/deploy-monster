package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// EnvImportHandler handles bulk .env file import/export.
type EnvImportHandler struct {
	store core.Store
}

func NewEnvImportHandler(store core.Store) *EnvImportHandler {
	return &EnvImportHandler{store: store}
}

// Import handles POST /api/v1/apps/{id}/env/import
// Accepts .env file format (KEY=VALUE per line) or JSON array.
func (h *EnvImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var vars []envVarEntry

	ct := r.Header.Get("Content-Type")
	if ct == "application/json" {
		if err := json.Unmarshal(body, &vars); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON array")
			return
		}
	} else {
		// Parse .env format
		vars = parseDotEnv(string(body))
	}

	if len(vars) == 0 {
		writeError(w, http.StatusBadRequest, "no variables found")
		return
	}

	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	data, _ := json.Marshal(vars)
	app.EnvVarsEnc = string(data)
	h.store.UpdateApp(r.Context(), app)

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":   appID,
		"imported": len(vars),
		"status":   "imported",
	})
}

// Export handles GET /api/v1/apps/{id}/env/export
// Returns env vars as .env file format.
func (h *EnvImportHandler) Export(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var vars []envVarEntry
	if app.EnvVarsEnc != "" {
		json.Unmarshal([]byte(app.EnvVarsEnc), &vars)
	}

	format := r.URL.Query().Get("format")
	if format == "json" {
		writeJSON(w, http.StatusOK, vars)
		return
	}

	// .env format
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", "attachment; filename=.env")
	for _, v := range vars {
		w.Write([]byte(v.Key + "=" + v.Value + "\n"))
	}
}

func parseDotEnv(content string) []envVarEntry {
	var vars []envVarEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// Remove surrounding quotes
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		vars = append(vars, envVarEntry{Key: key, Value: value})
	}
	return vars
}
