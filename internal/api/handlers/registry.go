package handlers

import (
	"log/slog"
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

func registryListKey(tenantID string) string {
	return "tenant:" + tenantID
}

func registryCredKey(tenantID, registryID string) string {
	return tenantID + ":" + registryID
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
func (h *RegistryHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	all := make([]RegistryConfig, len(builtinRegistries))
	copy(all, builtinRegistries)

	// Load custom registries from KV storage.
	var list registryList
	if err := h.bolt.Get("registries", registryListKey(claims.TenantID), &list); err == nil {
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
	if !decodeJSONInto(w, r, &req) {
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
	_ = h.bolt.Get("registries", registryListKey(claims.TenantID), &list)

	if len(list.Registries) >= 20 {
		writeError(w, http.StatusConflict, "registry limit reached (20)")
		return
	}
	list.Registries = append(list.Registries, newReg)

	if err := h.bolt.Set("registries", registryListKey(claims.TenantID), list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save registry")
		return
	}

	// Store credentials separately (password never in the list response)
	if req.Password != "" {
		if err := h.bolt.Set("registry_creds", registryCredKey(claims.TenantID, newReg.ID), map[string]string{
			"username": req.Username,
			"password": req.Password,
		}, 0); err != nil {
			slog.Error("failed to store registry credentials", "registry_id", newReg.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   newReg.ID,
		"name": newReg.Name,
		"url":  newReg.URL,
	})
}
