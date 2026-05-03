package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BackupHandler manages backup operations.
type BackupHandler struct {
	store   core.Store
	storage core.BackupStorage
	events  *core.EventBus
}

func NewBackupHandler(store core.Store, storage core.BackupStorage, events *core.EventBus) *BackupHandler {
	return &BackupHandler{store: store, storage: storage, events: events}
}

type createBackupRequest struct {
	SourceType string `json:"source_type"` // volume, database, config, full
	SourceID   string `json:"source_id"`
	Storage    string `json:"storage"` // local, s3
}

// List handles GET /api/v1/backups
func (h *BackupHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.storage == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	entries, err := h.storage.List(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "total": len(entries)})
}

// Create handles POST /api/v1/backups
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

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "queued",
		"message": "backup job has been queued",
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
	key, ok := requirePathParam(w, r, "key")
	if !ok {
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
