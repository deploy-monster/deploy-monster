package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
		Variables:  map[string]string{"NODE_ENV": "development", "LOG_LEVEL": "debug", "DEBUG": "true"},
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
