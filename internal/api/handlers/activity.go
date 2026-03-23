package handlers

import (
	"net/http"
	"strconv"

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

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	entries, total, err := h.store.ListAuditLogs(r.Context(), claims.TenantID, limit, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  entries,
		"total": total,
	})
}
