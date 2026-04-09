package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DBPoolHandler manages database connection pool configuration.
type DBPoolHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewDBPoolHandler(store core.Store, bolt core.BoltStorer) *DBPoolHandler {
	return &DBPoolHandler{store: store, bolt: bolt}
}

// PoolConfig holds database connection pool settings.
type PoolConfig struct {
	MaxConnections int `json:"max_connections"`
	MinConnections int `json:"min_connections"`
	IdleTimeout    int `json:"idle_timeout_sec"`
	MaxLifetime    int `json:"max_lifetime_sec"`
}

// defaultPoolConfig returns sensible defaults for a connection pool.
func defaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnections: 20,
		MinConnections: 2,
		IdleTimeout:    300,
		MaxLifetime:    3600,
	}
}

// Get handles GET /api/v1/databases/{id}/pool
func (h *DBPoolHandler) Get(w http.ResponseWriter, r *http.Request) {
	dbID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var cfg PoolConfig
	if err := h.bolt.Get("dbpool", dbID, &cfg); err != nil {
		// Return defaults if no config stored
		writeJSON(w, http.StatusOK, defaultPoolConfig())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/databases/{id}/pool
func (h *DBPoolHandler) Update(w http.ResponseWriter, r *http.Request) {
	dbID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var cfg PoolConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 20
	}
	if cfg.MinConnections <= 0 {
		cfg.MinConnections = 2
	}

	if err := h.bolt.Set("dbpool", dbID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save pool config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"db_id": dbID, "config": cfg, "status": "updated"})
}
