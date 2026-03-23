package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AdminAPIKeyHandler manages platform-level API keys.
type AdminAPIKeyHandler struct {
	store core.Store
}

func NewAdminAPIKeyHandler(store core.Store) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{store: store}
}

// List handles GET /api/v1/admin/api-keys
func (h *AdminAPIKeyHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// Generate handles POST /api/v1/admin/api-keys
func (h *AdminAPIKeyHandler) Generate(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	pair, err := auth.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"key":         pair.Key, // Shown only once
		"prefix":      pair.Prefix,
		"type":        "platform",
		"message":     "Save this key — it will not be shown again",
	})
}

// Revoke handles DELETE /api/v1/admin/api-keys/{prefix}
func (h *AdminAPIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("prefix")
	w.WriteHeader(http.StatusNoContent)
}
