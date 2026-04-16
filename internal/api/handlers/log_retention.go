package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LogRetentionHandler manages per-app log retention settings.
type LogRetentionHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewLogRetentionHandler(store core.Store, bolt core.BoltStorer) *LogRetentionHandler {
	return &LogRetentionHandler{store: store, bolt: bolt}
}

// LogRetentionConfig defines how long to keep container logs.
type LogRetentionConfig struct {
	MaxSizeMB int    `json:"max_size_mb"` // Max log file size before rotation
	MaxFiles  int    `json:"max_files"`   // Number of rotated files to keep
	Driver    string `json:"driver"`      // json-file, local, syslog
}

// defaultLogRetention returns sensible defaults.
func defaultLogRetention() LogRetentionConfig {
	return LogRetentionConfig{
		MaxSizeMB: 50,
		MaxFiles:  5,
		Driver:    "json-file",
	}
}

// Get handles GET /api/v1/apps/{id}/log-retention
func (h *LogRetentionHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg LogRetentionConfig
	if err := h.bolt.Get("log_retention", app.ID, &cfg); err != nil {
		// Return defaults if not configured
		writeJSON(w, http.StatusOK, defaultLogRetention())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/log-retention
func (h *LogRetentionHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

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
	if cfg.Driver == "" {
		cfg.Driver = "json-file"
	}
	const maxLogSizeMB = 10240
	const maxLogFiles = 100
	if cfg.MaxSizeMB > maxLogSizeMB {
		writeError(w, http.StatusBadRequest, "max_size_mb exceeds 10240 (10 GB)")
		return
	}
	if cfg.MaxFiles > maxLogFiles {
		writeError(w, http.StatusBadRequest, "max_files exceeds 100")
		return
	}
	switch cfg.Driver {
	case "json-file", "local", "syslog":
	default:
		writeError(w, http.StatusBadRequest, "driver must be one of: json-file, local, syslog")
		return
	}

	if err := h.bolt.Set("log_retention", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save log retention config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
