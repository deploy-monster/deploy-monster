package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── envvars.go ────────────────────
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

// ──────────────────── env_compare.go ────────────────────
// EnvCompareHandler compares environment variables between apps or projects.
type EnvCompareHandler struct {
	store core.Store
}

func NewEnvCompareHandler(store core.Store) *EnvCompareHandler {
	return &EnvCompareHandler{store: store}
}

// EnvDiff represents a difference between two env sets.
type EnvDiff struct {
	Key    string `json:"key"`
	Left   string `json:"left,omitempty"`
	Right  string `json:"right,omitempty"`
	Status string `json:"status"` // added, removed, changed, same
}

// Compare handles POST /api/v1/apps/env/compare
func (h *EnvCompareHandler) Compare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LeftAppID  string `json:"left_app_id"`
		RightAppID string `json:"right_app_id"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	leftApp, err := h.store.GetApp(r.Context(), req.LeftAppID)
	if err != nil || leftApp.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "left app not found")
		return
	}
	rightApp, err := h.store.GetApp(r.Context(), req.RightAppID)
	if err != nil || rightApp.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "right app not found")
		return
	}
	leftVars := parseEnvJSON(leftApp.EnvVarsEnc)
	rightVars := parseEnvJSON(rightApp.EnvVarsEnc)
	var diffs []EnvDiff
	seen := make(map[string]bool)
	for k, v := range leftVars {
		seen[k] = true
		rv, exists := rightVars[k]
		if !exists {
			diffs = append(diffs, EnvDiff{Key: k, Left: maskShort(v), Status: "removed"})
		} else if v != rv {
			diffs = append(diffs, EnvDiff{Key: k, Left: maskShort(v), Right: maskShort(rv), Status: "changed"})
		}
	}
	for k, v := range rightVars {
		if !seen[k] {
			diffs = append(diffs, EnvDiff{Key: k, Right: maskShort(v), Status: "added"})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"left":  req.LeftAppID,
		"right": req.RightAppID,
		"diffs": diffs,
		"total": len(diffs),
	})
}
func parseEnvJSON(enc string) map[string]string {
	result := make(map[string]string)
	if enc == "" {
		return result
	}
	var vars []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	json.Unmarshal([]byte(enc), &vars)
	for _, v := range vars {
		result[v.Key] = v.Value
	}
	return result
}
func maskShort(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + "**" + v[len(v)-2:]
}

// ──────────────────── env_import.go ────────────────────
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
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
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
	// Validate keys and sizes (same limits as envvars handler)
	const maxKeyLen = 256
	const maxValueLen = 64 * 1024  // 64 KB per value
	const maxTotalLen = 512 * 1024 // 512 KB total payload
	var totalSize int
	for _, v := range vars {
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
		val := sanitizeEnvValue(v.Value)
		w.Write([]byte(v.Key + "=" + val + "\n"))
	}
}

// sanitizeEnvValue quotes and escapes a .env value to prevent injection.
// It wraps the value in double quotes and escapes \, ", and $ within.
func sanitizeEnvValue(value string) string {
	var result strings.Builder
	result.WriteByte('"')
	for _, ch := range value {
		switch ch {
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		case '$':
			result.WriteString("$$")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		default:
			result.WriteRune(ch)
		}
	}
	result.WriteByte('"')
	return result.String()
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

// ──────────────────── environments.go ────────────────────
// EnvironmentHandler manages project environments (production, staging, dev).
type EnvironmentHandler struct {
	store core.Store
}

func NewEnvironmentHandler(store core.Store) *EnvironmentHandler {
	return &EnvironmentHandler{store: store}
}

// EnvironmentPreset defines a standard environment configuration.
type EnvironmentPreset struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Variables   map[string]string `json:"variables"`
	AutoDeploy  bool              `json:"auto_deploy"`
	Branch      string            `json:"branch"`
}

var defaultPresets = []EnvironmentPreset{
	{
		Name: "production", Description: "Live production environment",
		Variables:  map[string]string{"NODE_ENV": "production", "LOG_LEVEL": "warn"},
		AutoDeploy: false, Branch: "main",
	},
	{
		Name: "staging", Description: "Pre-production testing",
		Variables:  map[string]string{"NODE_ENV": "staging", "LOG_LEVEL": "info"},
		AutoDeploy: true, Branch: "staging",
	},
	{
		Name: "development", Description: "Development and testing",
		// DEBUG deliberately omitted: enabling debug mode globally can leak
		// sensitive data (stack traces, env dump, query details) into logs and
		// HTTP responses. Users who need it can add DEBUG=true via env vars UI.
		Variables:  map[string]string{"NODE_ENV": "development", "LOG_LEVEL": "debug"},
		AutoDeploy: true, Branch: "develop",
	},
}

// ListPresets handles GET /api/v1/environments/presets
func (h *EnvironmentHandler) ListPresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": defaultPresets})
}

// ApplyPreset handles POST /api/v1/projects/{id}/environment
func (h *EnvironmentHandler) ApplyPreset(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Environment string `json:"environment"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	project, err := h.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	// Verify project belongs to the caller's tenant (prevents cross-tenant access)
	if project.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	project.Environment = req.Environment
	// Would update project in DB
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":  projectID,
		"environment": req.Environment,
	})
}
