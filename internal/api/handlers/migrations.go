package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// MigrationHandler shows database migration status.
type MigrationHandler struct {
	core *core.Core
}

func NewMigrationHandler(c *core.Core) *MigrationHandler {
	return &MigrationHandler{core: c}
}

// Status handles GET /api/v1/admin/db/migrations. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *MigrationHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.core.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}

	migrations, err := h.core.Store.ListMigrations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"migrations": migrations,
		"total":      len(migrations),
		"driver":     h.core.Config.Database.Driver,
	})
}
