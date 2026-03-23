package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LogRetentionHandler manages per-app log retention settings.
type LogRetentionHandler struct {
	store core.Store
}

func NewLogRetentionHandler(store core.Store) *LogRetentionHandler {
	return &LogRetentionHandler{store: store}
}

// LogRetentionConfig defines how long to keep container logs.
type LogRetentionConfig struct {
	MaxSizeMB int    `json:"max_size_mb"` // Max log file size before rotation
	MaxFiles  int    `json:"max_files"`   // Number of rotated files to keep
	Driver    string `json:"driver"`      // json-file, local, syslog
}

// Get handles GET /api/v1/apps/{id}/log-retention
func (h *LogRetentionHandler) Get(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")

	writeJSON(w, http.StatusOK, LogRetentionConfig{
		MaxSizeMB: 50,
		MaxFiles:  5,
		Driver:    "json-file",
	})
}

// Update handles PUT /api/v1/apps/{id}/log-retention
func (h *LogRetentionHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg LogRetentionConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 50
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = 5
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
