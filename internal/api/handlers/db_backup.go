package handlers

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DBBackupHandler manages SQLite database backup/export.
type DBBackupHandler struct {
	core *core.Core
}

func NewDBBackupHandler(c *core.Core) *DBBackupHandler {
	return &DBBackupHandler{core: c}
}

// Backup handles GET /api/v1/admin/db/backup
// Downloads the current SQLite database file.
func (h *DBBackupHandler) Backup(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	dbPath := h.core.Config.Database.Path
	if dbPath == "" {
		dbPath = "deploymonster.db"
	}

	f, err := os.Open(dbPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database file not accessible")
		return
	}
	defer f.Close()

	info, _ := f.Stat()
	filename := fmt.Sprintf("deploymonster-backup-%s.db", time.Now().Format("20060102-150405"))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	ctx := r.Context()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := f.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// Status handles GET /api/v1/admin/db/status
func (h *DBBackupHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	dbPath := h.core.Config.Database.Path
	if dbPath == "" {
		dbPath = "deploymonster.db"
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database not accessible")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"driver":   h.core.Config.Database.Driver,
		"path":     dbPath,
		"size_mb":  info.Size() / 1024 / 1024,
		"modified": info.ModTime().Format(time.RFC3339),
	})
}
