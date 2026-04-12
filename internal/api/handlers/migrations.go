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
	if h.core.DB == nil || h.core.DB.SQL == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}

	rows, err := h.core.DB.SQL.QueryContext(r.Context(),
		"SELECT version, name, applied_at FROM _migrations ORDER BY version")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type migration struct {
		Version   int    `json:"version"`
		Name      string `json:"name"`
		AppliedAt string `json:"applied_at"`
	}

	var migrations []migration
	for rows.Next() {
		var m migration
		rows.Scan(&m.Version, &m.Name, &m.AppliedAt)
		migrations = append(migrations, m)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"migrations": migrations,
		"total":      len(migrations),
		"driver":     h.core.Config.Database.Driver,
	})
}
