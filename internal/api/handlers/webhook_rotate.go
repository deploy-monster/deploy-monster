package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const webhookSecretsBucket = "webhooks"

// WebhookRotateHandler rotates webhook signing secrets.
type WebhookRotateHandler struct {
	store  core.Store
	events *core.EventBus
	bolt   core.BoltStorer
}

func NewWebhookRotateHandler(store core.Store, events *core.EventBus, bolt core.BoltStorer) *WebhookRotateHandler {
	return &WebhookRotateHandler{store: store, events: events, bolt: bolt}
}

type webhookSecretRecord struct {
	ID         string    `json:"id"`
	AppID      string    `json:"app_id"`
	SecretHash string    `json:"secret_hash"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Rotate handles POST /api/v1/apps/{id}/webhooks/rotate
// Generates a new webhook secret and returns it (shown once).
func (h *WebhookRotateHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	newSecret := core.GenerateSecret(32)
	if err := h.persistSecret(app, newSecret); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rotate webhook secret")
		return
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("webhook.rotated", "api",
		map[string]string{"app_id": app.ID}))

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":     app.ID,
		"new_secret": newSecret,
		"message":    "Webhook secret rotated. Update your Git provider's webhook configuration.",
	})
}

func (h *WebhookRotateHandler) persistSecret(app *core.Application, secret string) error {
	if h.bolt == nil {
		return errors.New("webhook secret store not configured")
	}

	var rec webhookSecretRecord
	if err := h.bolt.Get(webhookSecretsBucket, app.ID, &rec); err != nil && !errors.Is(err, core.ErrKVNotFound) {
		return err
	}

	now := time.Now().UTC()
	if rec.ID == "" {
		rec.ID = app.ID
		rec.CreatedAt = now
	}
	rec.AppID = app.ID
	rec.SecretHash = secret
	rec.Status = "active"
	rec.UpdatedAt = now

	return h.bolt.Set(webhookSecretsBucket, app.ID, rec, 0)
}
