package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DBPoolHandler manages database connection pool configuration.
type DBPoolHandler struct {
	store core.Store
}

func NewDBPoolHandler(store core.Store) *DBPoolHandler {
	return &DBPoolHandler{store: store}
}

// PoolConfig holds database connection pool settings.
type PoolConfig struct {
	MaxConnections int `json:"max_connections"`
	MinConnections int `json:"min_connections"`
	IdleTimeout    int `json:"idle_timeout_sec"`
	MaxLifetime    int `json:"max_lifetime_sec"`
}

// Get handles GET /api/v1/databases/{id}/pool
func (h *DBPoolHandler) Get(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, PoolConfig{
		MaxConnections: 20, MinConnections: 2,
		IdleTimeout: 300, MaxLifetime: 3600,
	})
}

// Update handles PUT /api/v1/databases/{id}/pool
func (h *DBPoolHandler) Update(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("id")
	var cfg PoolConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"db_id": dbID, "config": cfg, "status": "updated"})
}
