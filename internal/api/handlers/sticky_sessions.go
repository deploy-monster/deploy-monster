package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// StickySessionHandler configures session affinity for load-balanced apps.
type StickySessionHandler struct {
	bolt core.BoltStorer
}

func NewStickySessionHandler(bolt core.BoltStorer) *StickySessionHandler {
	return &StickySessionHandler{bolt: bolt}
}

// StickySessionConfig holds cookie-based session affinity settings.
type StickySessionConfig struct {
	Enabled  bool   `json:"enabled"`
	Cookie   string `json:"cookie"`  // Cookie name (default: MONSTER_AFFINITY)
	MaxAge   int    `json:"max_age"` // Seconds
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"http_only"`
	SameSite string `json:"same_site"` // lax, strict, none
}

// defaultStickyConfig returns secure defaults.
func defaultStickyConfig() StickySessionConfig {
	return StickySessionConfig{
		Enabled: false, Cookie: "MONSTER_AFFINITY", MaxAge: 3600,
		Secure: true, HTTPOnly: true, SameSite: "lax",
	}
}

// Get handles GET /api/v1/apps/{id}/sticky-sessions
func (h *StickySessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg StickySessionConfig
	if err := h.bolt.Get("sticky_sessions", appID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultStickyConfig())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/sticky-sessions
func (h *StickySessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg StickySessionConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.Cookie == "" {
		cfg.Cookie = "MONSTER_AFFINITY"
	}

	if err := h.bolt.Set("sticky_sessions", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save sticky session config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
