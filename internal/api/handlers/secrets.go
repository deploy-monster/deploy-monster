package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SecretHandler manages encrypted secret storage.
type SecretHandler struct {
	store  core.Store
	vault  interface{ Encrypt(string) (string, error); Decrypt(string) (string, error) }
	events *core.EventBus
}

func NewSecretHandler(store core.Store, vault interface{ Encrypt(string) (string, error); Decrypt(string) (string, error) }, events *core.EventBus) *SecretHandler {
	return &SecretHandler{store: store, vault: vault, events: events}
}

type createSecretRequest struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Scope       string `json:"scope"`       // global, tenant, project, app
	Description string `json:"description"`
	ProjectID   string `json:"project_id,omitempty"`
	AppID       string `json:"app_id,omitempty"`
}

// Create handles POST /api/v1/secrets
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Value == "" {
		writeError(w, http.StatusBadRequest, "name and value are required")
		return
	}

	scope := req.Scope
	if scope == "" {
		scope = "tenant"
	}

	// Encrypt the value
	encrypted := req.Value
	if h.vault != nil {
		enc, err := h.vault.Encrypt(req.Value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
		encrypted = enc
	}

	// Store secret metadata in database
	secret := &core.Secret{
		TenantID:       claims.TenantID,
		ProjectID:      req.ProjectID,
		AppID:          req.AppID,
		Name:           req.Name,
		Type:           "env_var",
		Description:    req.Description,
		Scope:          scope,
		CurrentVersion: 1,
	}
	if err := h.store.CreateSecret(r.Context(), secret); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store secret")
		return
	}

	// Store encrypted version
	version := &core.SecretVersion{
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  encrypted,
		CreatedBy: claims.UserID,
	}
	if err := h.store.CreateSecretVersion(r.Context(), version); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store secret version")
		return
	}

	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventSecretCreated, "api", claims.TenantID, claims.UserID,
		map[string]string{"name": req.Name, "scope": scope},
	))

	writeJSON(w, http.StatusCreated, map[string]any{
		"name":        req.Name,
		"scope":       scope,
		"description": req.Description,
		"encrypted":   len(encrypted) > 0,
		"reference":   "${SECRET:" + req.Name + "}",
	})
}

// List handles GET /api/v1/secrets
// Returns secret names and metadata — never the actual values.
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	secrets, err := h.store.ListSecretsByTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list secrets")
		return
	}

	// Return metadata only — never expose encrypted values
	data := make([]map[string]any, 0, len(secrets))
	for _, s := range secrets {
		data = append(data, map[string]any{
			"id":          s.ID,
			"name":        s.Name,
			"scope":       s.Scope,
			"description": s.Description,
			"reference":   "${SECRET:" + s.Name + "}",
			"version":     s.CurrentVersion,
			"created_at":  s.CreatedAt,
			"updated_at":  s.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  data,
		"total": len(data),
	})
}
