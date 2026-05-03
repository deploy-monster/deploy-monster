package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AdminAPIKeyHandler manages platform-level API keys.
type AdminAPIKeyHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewAdminAPIKeyHandler(store core.Store, bolt core.BoltStorer) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{store: store, bolt: bolt}
}

// apiKeyRecord is persisted in BBolt for each API key.
type apiKeyRecord struct {
	Prefix    string     `json:"prefix"`
	Hash      string     `json:"hash"`
	Type      string     `json:"type"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // nil = no expiry
}

type apiKeyListItem struct {
	Prefix    string     `json:"prefix"`
	Type      string     `json:"type"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// apiKeyIndex maintains the list of all active API key prefixes.
type apiKeyIndex struct {
	Prefixes []string `json:"prefixes"`
}

// List handles GET /api/v1/admin/api-keys
func (h *AdminAPIKeyHandler) List(w http.ResponseWriter, _ *http.Request) {
	var idx apiKeyIndex
	if err := h.bolt.Get("api_keys", "_index", &idx); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	keys := make([]apiKeyListItem, 0, len(idx.Prefixes))
	for _, prefix := range idx.Prefixes {
		var rec apiKeyRecord
		if err := h.bolt.Get("api_keys", prefix, &rec); err == nil {
			keys = append(keys, apiKeyListItem{
				Prefix:    rec.Prefix,
				Type:      rec.Type,
				CreatedBy: rec.CreatedBy,
				CreatedAt: rec.CreatedAt,
				ExpiresAt: rec.ExpiresAt,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": keys, "total": len(keys)})
}

// Generate handles POST /api/v1/admin/api-keys.
// Authorization is enforced by middleware.RequireSuperAdmin at the router
// level; by the time this runs, claims are guaranteed present and role is
// role_super_admin. The claims fetch below is only for created_by tagging.
func (h *AdminAPIKeyHandler) Generate(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())

	pair, err := auth.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}

	rec := apiKeyRecord{
		Prefix:    pair.Prefix,
		Hash:      pair.Hash,
		Type:      "platform",
		CreatedBy: claims.UserID,
		CreatedAt: time.Now(),
	}

	// Store the key record
	if err := h.bolt.Set("api_keys", pair.Prefix, rec, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store api key")
		return
	}

	// Update the index
	var idx apiKeyIndex
	_ = h.bolt.Get("api_keys", "_index", &idx)
	idx.Prefixes = append(idx.Prefixes, pair.Prefix)
	if err := h.bolt.Set("api_keys", "_index", idx, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update key index")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"key":     pair.Key, // Shown only once
		"prefix":  pair.Prefix,
		"type":    "platform",
		"message": "Save this key — it will not be shown again",
	})
}

// CleanupExpiredKeys removes API keys that have passed their expiry time.
// Safe to call periodically from a background scheduler.
func (h *AdminAPIKeyHandler) CleanupExpiredKeys() int {
	var idx apiKeyIndex
	if err := h.bolt.Get("api_keys", "_index", &idx); err != nil {
		return 0
	}

	now := time.Now()
	var removed int
	active := make([]string, 0, len(idx.Prefixes))
	for _, prefix := range idx.Prefixes {
		var rec apiKeyRecord
		if err := h.bolt.Get("api_keys", prefix, &rec); err != nil {
			continue // key gone, skip
		}
		if rec.ExpiresAt != nil && now.After(*rec.ExpiresAt) {
			if err := h.bolt.Delete("api_keys", prefix); err != nil {
				slog.Error("failed to delete expired API key", "prefix", prefix, "error", err)
			}
			removed++
		} else {
			active = append(active, prefix)
		}
	}

	if removed > 0 {
		idx.Prefixes = active
		if err := h.bolt.Set("api_keys", "_index", idx, 0); err != nil {
			slog.Error("failed to update API key index after cleanup", "error", err)
		}
	}
	return removed
}

// Revoke handles DELETE /api/v1/admin/api-keys/{prefix}.
// Authorized by middleware.RequireSuperAdmin at the router.
func (h *AdminAPIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	prefix, ok := requirePathParam(w, r, "prefix")
	if !ok {
		return
	}

	// Delete the key record
	_ = h.bolt.Delete("api_keys", prefix)

	// Update the index
	var idx apiKeyIndex
	if err := h.bolt.Get("api_keys", "_index", &idx); err == nil {
		filtered := make([]string, 0, len(idx.Prefixes))
		for _, p := range idx.Prefixes {
			if p != prefix {
				filtered = append(filtered, p)
			}
		}
		idx.Prefixes = filtered
		_ = h.bolt.Set("api_keys", "_index", idx, 0)
	}

	w.WriteHeader(http.StatusNoContent)
}
