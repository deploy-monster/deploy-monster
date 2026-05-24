package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DBBackupHandler manages SQLite database backup/export.
type DBBackupHandler struct {
	core *core.Core
}

func NewDBBackupHandler(c *core.Core) *DBBackupHandler {
	return &DBBackupHandler{core: c}
}

// Backup handles GET /api/v1/admin/db/backup. Downloads the current
// SQLite database file. Authorization is enforced by
// middleware.RequireSuperAdmin at the router.
func (h *DBBackupHandler) Backup(w http.ResponseWriter, r *http.Request) {
	dbPath := h.core.Config.Database.Path
	if dbPath == "" {
		dbPath = "deploymonster.db"
	}

	filename := fmt.Sprintf("deploymonster-backup-%s.db", time.Now().Format("20060102-150405"))
	f, cleanup, err := h.openBackupFile(r, dbPath, filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database file not accessible")
		return
	}
	defer cleanup()
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database file not accessible")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(filename))
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

func (h *DBBackupHandler) openBackupFile(r *http.Request, dbPath, filename string) (*os.File, func(), error) {
	if h.core.DB != nil && h.core.DB.Snapshotter != nil {
		dir, err := os.MkdirTemp("", "deploymonster-db-backup-*")
		if err != nil {
			return nil, func() {}, err
		}
		cleanup := func() { _ = os.RemoveAll(dir) }
		snapshotPath := filepath.Join(dir, filename)
		if err := h.core.DB.Snapshotter.SnapshotBackup(r.Context(), snapshotPath); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		f, err := os.Open(snapshotPath)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		return f, cleanup, nil
	}

	f, err := os.Open(dbPath)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() {}, nil
}

// Status handles GET /api/v1/admin/db/status. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *DBBackupHandler) Status(w http.ResponseWriter, r *http.Request) {
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
