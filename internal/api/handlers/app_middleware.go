package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AppMiddlewareHandler configures per-app ingress middleware.
type AppMiddlewareHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewAppMiddlewareHandler(store core.Store, bolt core.BoltStorer) *AppMiddlewareHandler {
	return &AppMiddlewareHandler{store: store, bolt: bolt}
}

// MiddlewareConfig defines which middleware are active for an app.
type MiddlewareConfig struct {
	RateLimit *RateLimitMiddleware `json:"rate_limit,omitempty"`
	CORS      *CORSMiddleware      `json:"cors,omitempty"`
	Compress  bool                 `json:"compress"`
	Headers   map[string]string    `json:"headers,omitempty"`
}

// RateLimitMiddleware config for per-app rate limiting.
type RateLimitMiddleware struct {
	Enabled        bool   `json:"enabled"`
	RequestsPerMin int    `json:"requests_per_min"`
	BurstSize      int    `json:"burst_size"`
	By             string `json:"by"` // ip, header, path
}

// CORSMiddleware config for per-app CORS.
type CORSMiddleware struct {
	Enabled        bool     `json:"enabled"`
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
	MaxAge         int      `json:"max_age"`
}

// Get handles GET /api/v1/apps/{id}/middleware
func (h *AppMiddlewareHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg MiddlewareConfig
	if err := h.bolt.Get("app_middleware", appID, &cfg); err != nil {
		// Return default config
		writeJSON(w, http.StatusOK, MiddlewareConfig{Compress: true})
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/middleware
func (h *AppMiddlewareHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg MiddlewareConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.bolt.Set("app_middleware", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save middleware config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
