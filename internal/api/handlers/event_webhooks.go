package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// EventWebhookHandler manages outbound system event webhooks.
// When events occur (deploy, crash, alert), configured URLs receive notifications.
type EventWebhookHandler struct {
	store  core.Store
	events *core.EventBus
	bolt   core.BoltStorer
}

func NewEventWebhookHandler(store core.Store, events *core.EventBus, bolt core.BoltStorer) *EventWebhookHandler {
	return &EventWebhookHandler{store: store, events: events, bolt: bolt}
}

// EventWebhookConfig represents an outbound event webhook.
type EventWebhookConfig struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	SecretHash string   `json:"secret_hash,omitempty"` // SHA-256 hash of secret (not the secret itself)
	Events     []string `json:"events"`                // app.deployed, app.crashed, alert.triggered, etc.
	Active     bool     `json:"active"`
	TenantID   string   `json:"tenant_id,omitempty"` // Tenant that owns this webhook
}

// hashSecret creates a SHA-256 hash of a webhook secret for storage.
// The original secret cannot be recovered from the hash.
func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// checkSecret verifies if a provided secret matches the stored hash.
// Used during outbound webhook delivery to sign requests.
func checkSecret(provided, storedHash string) bool {
	h := hashSecret(provided)
	return h == storedHash
}

// webhookListKey returns the BBolt bucket key for a tenant's webhook list.
func webhookListKey(tenantID string) string {
	return "tenant:" + tenantID
}

// eventWebhookList wraps the persisted list of outbound webhook configs.
type eventWebhookList struct {
	Webhooks []EventWebhookConfig `json:"webhooks"`
}

// List handles GET /api/v1/webhooks/outbound
func (h *EventWebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	pg := parsePagination(r)

	var list eventWebhookList
	key := webhookListKey(claims.TenantID)
	_ = h.bolt.Get("event_webhooks", key, &list)

	// Don't return secret hash to clients — webhooks are write-only
	safe := make([]EventWebhookConfig, len(list.Webhooks))
	for i, wh := range list.Webhooks {
		safe[i] = wh
		safe[i].SecretHash = "" // Strip hash from list response
	}

	paged, total := paginateSlice(safe, pg)
	writePaginatedJSON(w, paged, total, pg)
}

// Create handles POST /api/v1/webhooks/outbound
func (h *EventWebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret,omitempty"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.URL == "" || len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "url and events are required")
		return
	}
	if len(req.URL) > 2048 {
		writeError(w, http.StatusBadRequest, "url must be 2048 characters or less")
		return
	}
	if len(req.Events) > 50 {
		writeError(w, http.StatusBadRequest, "events list must have 50 entries or less")
		return
	}

	// Generate secret if not provided; this is returned once only at creation
	secret := req.Secret
	if secret == "" {
		secret = core.GenerateSecret(32)
	}

	wh := EventWebhookConfig{
		ID:         core.GenerateID(),
		URL:        req.URL,
		SecretHash: hashSecret(secret), // Store hash, not plaintext
		Events:     req.Events,
		Active:     true,
		TenantID:   claims.TenantID,
	}

	key := webhookListKey(claims.TenantID)
	var list eventWebhookList
	_ = h.bolt.Get("event_webhooks", key, &list)

	// Per-tenant limit: max 20 webhooks per tenant (prevents one tenant exhausting global limit)
	const maxWebhooksPerTenant = 20
	if len(list.Webhooks) >= maxWebhooksPerTenant {
		writeError(w, http.StatusConflict, "webhook limit reached (20 per tenant)")
		return
	}
	list.Webhooks = append(list.Webhooks, wh)

	if err := h.bolt.Set("event_webhooks", key, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save webhook config")
		return
	}

	// Return the config WITH the plaintext secret — client must save it
	// since it cannot be recovered from the stored hash.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          wh.ID,
		"url":         wh.URL,
		"secret":      secret, // Plaintext — shown only once at creation
		"events":      wh.Events,
		"active":      wh.Active,
		"secret_hash": "", // Never returned
	})
}

// Delete handles DELETE /api/v1/webhooks/outbound/{id}
func (h *EventWebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	key := webhookListKey(claims.TenantID)
	var list eventWebhookList
	if err := h.bolt.Get("event_webhooks", key, &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	filtered := make([]EventWebhookConfig, 0, len(list.Webhooks))
	for _, wh := range list.Webhooks {
		if wh.ID != id {
			filtered = append(filtered, wh)
		}
	}
	list.Webhooks = filtered

	if err := h.bolt.Set("event_webhooks", key, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook configs")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventEventWebhookDeleted, "api",
			map[string]string{"id": id}))
	}

	w.WriteHeader(http.StatusNoContent)
}
