package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BasicAuthHandler manages HTTP Basic Auth protection per app.
// When enabled, the ingress adds a Basic Auth challenge before proxying.
type BasicAuthHandler struct {
	store  core.Store
	bolt   core.BoltStorer
	events *core.EventBus
}

func NewBasicAuthHandler(store core.Store, bolt core.BoltStorer) *BasicAuthHandler {
	return &BasicAuthHandler{store: store, bolt: bolt}
}

// SetEvents sets the event bus for audit event emission.
func (h *BasicAuthHandler) SetEvents(events *core.EventBus) { h.events = events }

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
	if len(cfg.Realm) > 100 {
		writeError(w, http.StatusBadRequest, "realm must be 100 characters or less")
		return
	}
	if len(cfg.Users) > 50 {
		writeError(w, http.StatusBadRequest, "maximum 50 users allowed")
		return
	}
	for username := range cfg.Users {
		if len(username) > 100 {
			writeError(w, http.StatusBadRequest, "username must be 100 characters or less")
			return
		}
	}

	if err := h.bolt.Set("basic_auth", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save basic auth config")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventBasicAuthUpdated, "api",
			map[string]string{"app_id": appID, "enabled": fmt.Sprintf("%t", cfg.Enabled)}))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
