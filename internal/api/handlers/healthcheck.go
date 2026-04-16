package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// HealthCheckHandler manages per-app health check configuration.
type HealthCheckHandler struct {
	store core.Store
}

func NewHealthCheckHandler(store core.Store) *HealthCheckHandler {
	return &HealthCheckHandler{store: store}
}

// HealthCheckConfig defines how to check if an app is healthy.
type HealthCheckConfig struct {
	Type     string `json:"type"`     // http, tcp, exec, none
	Path     string `json:"path"`     // HTTP health check path
	Port     int    `json:"port"`     // Port to check
	Interval int    `json:"interval"` // Seconds between checks
	Timeout  int    `json:"timeout"`  // Seconds before timeout
	Retries  int    `json:"retries"`  // Failures before unhealthy
}

// Get handles GET /api/v1/apps/{id}/healthcheck
func (h *HealthCheckHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	// Default health check config
	writeJSON(w, http.StatusOK, HealthCheckConfig{
		Type:     "http",
		Path:     "/health",
		Port:     0, // Use app's primary port
		Interval: 10,
		Timeout:  5,
		Retries:  3,
	})
}

// Update handles PUT /api/v1/apps/{id}/healthcheck
func (h *HealthCheckHandler) Update(w http.ResponseWriter, r *http.Request) {
	// SECURITY: Verify the app belongs to this tenant
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg HealthCheckConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := map[string]bool{"http": true, "tcp": true, "exec": true, "none": true}
	if !valid[cfg.Type] {
		writeError(w, http.StatusBadRequest, "type must be: http, tcp, exec, none")
		return
	}

	// Cap Path so a malicious admin can't store a multi-MB string that
	// slips under the 10MB BodyLimit middleware and then gets reloaded on
	// every health probe.
	if len(cfg.Path) > 2048 {
		writeError(w, http.StatusBadRequest, "path must be 2048 characters or fewer")
		return
	}
	// 0 = use app's primary port; otherwise must be in TCP/UDP range.
	if cfg.Port < 0 || cfg.Port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be between 0 and 65535")
		return
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 10
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}
	// Upper bounds to prevent pathological values. A 1-hour interval is
	// the longest anyone reasonably wants; same for timeout. 100 retries
	// is more than any real workload needs and caps the worst-case
	// memory the health checker allocates per app.
	if cfg.Interval > 3600 {
		writeError(w, http.StatusBadRequest, "interval must be 3600 seconds or fewer")
		return
	}
	if cfg.Timeout > 300 {
		writeError(w, http.StatusBadRequest, "timeout must be 300 seconds or fewer")
		return
	}
	if cfg.Retries > 100 {
		writeError(w, http.StatusBadRequest, "retries must be 100 or fewer")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
