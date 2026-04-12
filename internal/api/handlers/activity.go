package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ActivityHandler serves tenant activity feed.
type ActivityHandler struct {
	store core.Store
}

func NewActivityHandler(store core.Store) *ActivityHandler {
	return &ActivityHandler{store: store}
}

// Feed handles GET /api/v1/activity
// Returns recent audit log entries as an activity feed.
func (h *ActivityHandler) Feed(w http.ResponseWriter, r *http.Request) {
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
