package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// validCookieName matches an RFC 6265 cookie-name (a token). Rejecting
// non-token characters is what prevents Set-Cookie header splitting via
// a user-supplied cookie name.
var validCookieName = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

// StickySessionHandler configures session affinity for load-balanced apps.
type StickySessionHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewStickySessionHandler(store core.Store, bolt core.BoltStorer) *StickySessionHandler {
	return &StickySessionHandler{store: store, bolt: bolt}
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
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg StickySessionConfig
	if err := h.bolt.Get("sticky_sessions", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultStickyConfig())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/sticky-sessions
func (h *StickySessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg StickySessionConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.Cookie == "" {
		cfg.Cookie = "MONSTER_AFFINITY"
	}
	// RFC 6265 token chars only — otherwise an attacker-controlled cookie
	// name ending in `; Path=/; Set-Cookie:` could split the Set-Cookie
	// header when the reverse proxy writes it.
	if !validCookieName.MatchString(cfg.Cookie) {
		writeError(w, http.StatusBadRequest, "cookie name must be RFC 6265 token characters only (alphanumerics, !#$%&'*+-.^_`|~)")
		return
	}
	if len(cfg.Cookie) > 128 {
		writeError(w, http.StatusBadRequest, "cookie name must be 128 characters or fewer")
		return
	}
	// 0 = session cookie; otherwise positive with a hard cap (1 year).
	if cfg.MaxAge < 0 {
		writeError(w, http.StatusBadRequest, "max_age must be zero or positive")
		return
	}
	if cfg.MaxAge > 31536000 {
		writeError(w, http.StatusBadRequest, "max_age must be 31536000 seconds (1 year) or fewer")
		return
	}
	if cfg.SameSite == "" {
		cfg.SameSite = "lax"
	}
	if cfg.SameSite != "lax" && cfg.SameSite != "strict" && cfg.SameSite != "none" {
		writeError(w, http.StatusBadRequest, "same_site must be one of: lax, strict, none")
		return
	}

	if err := h.bolt.Set("sticky_sessions", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save sticky session config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
