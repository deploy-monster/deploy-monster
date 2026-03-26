package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RegistryHandler manages Docker registry connections.
type RegistryHandler struct {
	bolt core.BoltStorer
}

func NewRegistryHandler(bolt core.BoltStorer) *RegistryHandler {
	return &RegistryHandler{bolt: bolt}
}

// RegistryConfig represents a Docker registry connection.
type RegistryConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"` // e.g., ghcr.io, registry.example.com
	Username string `json:"username"`
	Password string `json:"-"` // Never returned
	IsPublic bool   `json:"is_public"`
}

// registryList holds all configured registries.
type registryList struct {
	Registries []RegistryConfig `json:"registries"`
}

// builtinRegistries are always available.
var builtinRegistries = []RegistryConfig{
	{ID: "dockerhub", Name: "Docker Hub", URL: "docker.io", IsPublic: true},
	{ID: "ghcr", Name: "GitHub Container Registry", URL: "ghcr.io", IsPublic: true},
	{ID: "gcr", Name: "Google Container Registry", URL: "gcr.io", IsPublic: true},
}

// List handles GET /api/v1/registries
func (h *RegistryHandler) List(w http.ResponseWriter, _ *http.Request) {
	all := make([]RegistryConfig, len(builtinRegistries))
	copy(all, builtinRegistries)

	// Load custom registries from BBolt
	var list registryList
	if err := h.bolt.Get("registries", "all", &list); err == nil {
		all = append(all, list.Registries...)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": all, "total": len(all)})
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

	newReg := RegistryConfig{
		ID:       core.GenerateID(),
		Name:     req.Name,
		URL:      req.URL,
		Username: req.Username,
		IsPublic: false,
	}

	// Load existing custom registries
	var list registryList
	_ = h.bolt.Get("registries", "all", &list)

	list.Registries = append(list.Registries, newReg)

	if err := h.bolt.Set("registries", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save registry")
		return
	}

	// Store credentials separately (password never in the list response)
	if req.Password != "" {
		_ = h.bolt.Set("registry_creds", newReg.ID, map[string]string{
			"username": req.Username,
			"password": req.Password,
		}, 0)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   newReg.ID,
		"name": newReg.Name,
		"url":  newReg.URL,
	})
}
