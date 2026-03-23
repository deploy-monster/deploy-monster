package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RegistryHandler manages Docker registry connections.
type RegistryHandler struct {
	store core.Store
}

func NewRegistryHandler(store core.Store) *RegistryHandler {
	return &RegistryHandler{store: store}
}

// RegistryConfig represents a Docker registry connection.
type RegistryConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`      // e.g., ghcr.io, registry.example.com
	Username string `json:"username"`
	Password string `json:"-"` // Never returned
	IsPublic bool   `json:"is_public"`
}

// List handles GET /api/v1/registries
func (h *RegistryHandler) List(w http.ResponseWriter, _ *http.Request) {
	// Built-in registries always available
	registries := []map[string]any{
		{"id": "dockerhub", "name": "Docker Hub", "url": "docker.io", "is_public": true},
		{"id": "ghcr", "name": "GitHub Container Registry", "url": "ghcr.io", "is_public": true},
		{"id": "gcr", "name": "Google Container Registry", "url": "gcr.io", "is_public": true},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": registries, "total": len(registries)})
}

type addRegistryRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Add handles POST /api/v1/registries
func (h *RegistryHandler) Add(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req addRegistryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   core.GenerateID(),
		"name": req.Name,
		"url":  req.URL,
	})
}
