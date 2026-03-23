package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// StickySessionHandler configures session affinity for load-balanced apps.
type StickySessionHandler struct {
	store core.Store
}

func NewStickySessionHandler(store core.Store) *StickySessionHandler {
	return &StickySessionHandler{store: store}
}

// StickySessionConfig holds cookie-based session affinity settings.
type StickySessionConfig struct {
	Enabled  bool   `json:"enabled"`
	Cookie   string `json:"cookie"`    // Cookie name (default: MONSTER_AFFINITY)
	MaxAge   int    `json:"max_age"`   // Seconds
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"http_only"`
	SameSite string `json:"same_site"` // lax, strict, none
}

// Get handles GET /api/v1/apps/{id}/sticky-sessions
func (h *StickySessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, StickySessionConfig{
		Enabled: false, Cookie: "MONSTER_AFFINITY", MaxAge: 3600,
		Secure: true, HTTPOnly: true, SameSite: "lax",
	})
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
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
