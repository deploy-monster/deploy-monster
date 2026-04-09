package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BasicAuthHandler manages HTTP Basic Auth protection per app.
// When enabled, the ingress adds a Basic Auth challenge before proxying.
type BasicAuthHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewBasicAuthHandler(store core.Store, bolt core.BoltStorer) *BasicAuthHandler {
	return &BasicAuthHandler{store: store, bolt: bolt}
}

// BasicAuthConfig holds per-app basic auth settings.
type BasicAuthConfig struct {
	Enabled bool              `json:"enabled"`
	Users   map[string]string `json:"users"` // username -> bcrypt hash
	Realm   string            `json:"realm"` // Challenge realm text
}

// Get handles GET /api/v1/apps/{id}/basic-auth
func (h *BasicAuthHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg BasicAuthConfig
	if err := h.bolt.Get("basic_auth", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, BasicAuthConfig{Enabled: false, Realm: "Restricted"})
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/basic-auth
func (h *BasicAuthHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg BasicAuthConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.Realm == "" {
		cfg.Realm = "Restricted"
	}

	if err := h.bolt.Set("basic_auth", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save basic auth config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
