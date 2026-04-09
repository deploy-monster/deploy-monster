package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TeamHandler handles team management endpoints.
type TeamHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewTeamHandler(store core.Store, events *core.EventBus) *TeamHandler {
	return &TeamHandler{store: store, events: events}
}

// ListRoles handles GET /api/v1/team/roles
func (h *TeamHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roles, err := h.store.ListRoles(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": roles, "total": len(roles)})
}

// GetAuditLog handles GET /api/v1/team/audit-log
func (h *TeamHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	pg := parsePagination(r)

	entries, total, err := h.store.ListAuditLogs(r.Context(), claims.TenantID, pg.PerPage, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writePaginatedJSON(w, entries, total, pg)
}
