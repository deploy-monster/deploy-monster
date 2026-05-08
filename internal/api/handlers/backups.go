package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BackupTrigger is the surface the handler needs from the backup module
// to actually run a backup on demand. Module satisfies this interface;
// pulling it out as a small contract keeps the handler from importing the
// whole backup package and avoids an import cycle (backup already
// transitively depends on api/handlers via core).
type BackupTrigger interface {
	TriggerNow(ctx context.Context) error
}

// BackupHandler manages backup operations.
type BackupHandler struct {
	store   core.Store
	storage core.BackupStorage
	events  *core.EventBus
	trigger BackupTrigger
}

func NewBackupHandler(store core.Store, storage core.BackupStorage, events *core.EventBus) *BackupHandler {
	return &BackupHandler{store: store, storage: storage, events: events}
}

func tenantBackupPrefix(tenantID string) string {
	return strings.Trim(tenantID, "/") + "/"
}

func requireTenantBackupKey(w http.ResponseWriter, key, tenantID string) bool {
	key = strings.TrimSpace(key)
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		writeError(w, http.StatusBadRequest, "invalid backup key")
		return false
	}
	if !strings.HasPrefix(key, tenantBackupPrefix(tenantID)) {
		writeError(w, http.StatusNotFound, "backup not found")
		return false
	}
	return true
}

// SetTrigger wires the on-demand backup trigger. Called from the router
// after the backup module is registered in the module registry.
func (h *BackupHandler) SetTrigger(t BackupTrigger) { h.trigger = t }

type createBackupRequest struct {
	SourceType string `json:"source_type"` // volume, database, config, full
	SourceID   string `json:"source_id"`
	Storage    string `json:"storage"` // local, s3
}

// List handles GET /api/v1/backups
func (h *BackupHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.storage == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	entries, err := h.storage.List(r.Context(), tenantBackupPrefix(claims.TenantID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "total": len(entries)})
}

// Create handles POST /api/v1/backups. Triggers a real backup run if the
// backup module is wired; falls back to "queued" if no trigger is set so
// older deployments that haven't been re-wired don't 500 on every press.
func (h *BackupHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventBackupStarted, "api", claims.TenantID, claims.UserID,
		map[string]string{"source_type": req.SourceType, "source_id": req.SourceID},
	))

	if h.trigger == nil {
		writeError(w, http.StatusServiceUnavailable, "backup engine not ready")
		return
	}
	if err := h.trigger.TriggerNow(r.Context()); err != nil {
		internalErrorCtx(r.Context(), w, "failed to start backup", err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "running",
		"message": "backup job started — watch the activity feed for completion",
	})
}

// Restore handles POST /api/v1/backups/{key}/restore
func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	key, ok := requirePathParam(w, r, "key")
	if !ok {
		return
	}
	if !requireTenantBackupKey(w, key, claims.TenantID) {
		return
	}
	if h.storage == nil {
		writeError(w, http.StatusServiceUnavailable, "backup storage not configured")
		return
	}

	reader, err := h.storage.Download(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	reader.Close()

	h.events.Publish(r.Context(), core.NewTenantEvent(
		"backup.restore_started", "api", claims.TenantID, claims.UserID,
		map[string]string{"key": key},
	))

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "queued",
		"message": "restore job has been queued",
	})
}

// Download handles GET /api/v1/backups/{key}/download
func (h *BackupHandler) Download(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	key, ok := requirePathParam(w, r, "key")
	if !ok {
		return
	}
	if !requireTenantBackupKey(w, key, claims.TenantID) {
		return
	}
	if h.storage == nil {
		writeError(w, http.StatusServiceUnavailable, "backup storage not configured")
		return
	}

	reader, err := h.storage.Download(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(key))

	ctx := r.Context()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := reader.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
