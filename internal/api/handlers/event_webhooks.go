package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── event_webhooks.go ────────────────────
// EventWebhookHandler manages outbound system event webhooks.
// When events occur (deploy, crash, alert), configured URLs receive notifications.
type EventWebhookHandler struct {
	store  core.Store
	events *core.EventBus
	kv     core.KVStorer
}

func NewEventWebhookHandler(store core.Store, events *core.EventBus, kv core.KVStorer) *EventWebhookHandler {
	return &EventWebhookHandler{store: store, events: events, kv: kv}
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

// webhookListKey returns the KV bucket key for a tenant's webhook list.
func webhookListKey(tenantID string) string {
	return "tenant:" + tenantID
}

// validateWebhookURL validates that a webhook URL is safe to call.
// It blocks localhost, private/internal IPs, non-HTTPS schemes, and cloud metadata endpoints.
func validateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	// Only allow HTTPS URLs
	if u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use HTTPS scheme")
	}
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("webhook URL must have a hostname")
	}
	// Block localhost variants
	localhostVariants := []string{"localhost", "127.0.0.1", "::1", "0.0.0.0", "[::1]"}
	for _, variant := range localhostVariants {
		if strings.EqualFold(hostname, variant) {
			return fmt.Errorf("webhook URL cannot point to localhost")
		}
	}
	// Block private, loopback, link-local, multicast IPs
	ip := net.ParseIP(hostname)
	if ip != nil {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return fmt.Errorf("webhook URL cannot point to internal IP addresses")
		}
		// Block cloud metadata endpoints
		if ip.String() == "169.254.169.254" || ip.String() == "169.254.169.253" {
			return fmt.Errorf("webhook URL cannot point to cloud metadata endpoints")
		}
	}
	// Block common internal hostnames
	internalHostnames := []string{
		"metadata.google.internal",
		"metadata",
		"metadata.ec2.internal",
		"kubernetes.default",
		"kubernetes.default.svc",
		"kubernetes.default.svc.cluster.local",
	}
	for _, internal := range internalHostnames {
		if strings.EqualFold(hostname, internal) || strings.HasSuffix(strings.ToLower(hostname), "."+strings.ToLower(internal)) {
			return fmt.Errorf("webhook URL cannot point to internal hostnames")
		}
	}
	return nil
}

// eventWebhookList wraps the persisted list of outbound webhook configs.
type eventWebhookList struct {
	Webhooks []EventWebhookConfig `json:"webhooks"`
}

var (
	errWebhookLimitReached = errors.New("webhook limit reached")
	errWebhookListMissing  = errors.New("webhook list missing")
)

type kvValueMutator interface {
	Mutate(bucket, key string, dest any, ttlSeconds int64, mutate func(exists bool) error) error
}

func mutateKVValue(kv core.KVStorer, bucket, key string, dest any, ttlSeconds int64, mutate func(exists bool) error) error {
	if mutator, ok := kv.(kvValueMutator); ok {
		return mutator.Mutate(bucket, key, dest, ttlSeconds, mutate)
	}
	exists := true
	if err := kv.Get(bucket, key, dest); err != nil {
		if !errors.Is(err, core.ErrKVNotFound) {
			return err
		}
		exists = false
	}
	if err := mutate(exists); err != nil {
		return err
	}
	return kv.Set(bucket, key, dest, ttlSeconds)
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
	_ = h.kv.Get("event_webhooks", key, &list)
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
	if !decodeJSONInto(w, r, &req) {
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
	// Validate webhook URL — block private IPs, localhost, and non-HTTPS
	if err := validateWebhookURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	// Per-tenant limit: max 20 webhooks per tenant (prevents one tenant exhausting global limit)
	const maxWebhooksPerTenant = 20
	err := mutateKVValue(h.kv, "event_webhooks", key, &list, 0, func(_ bool) error {
		if len(list.Webhooks) >= maxWebhooksPerTenant {
			return errWebhookLimitReached
		}
		list.Webhooks = append(list.Webhooks, wh)
		return nil
	})
	if errors.Is(err, errWebhookLimitReached) {
		writeError(w, http.StatusConflict, "webhook limit reached (20 per tenant)")
		return
	}
	if err != nil {
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
	err := mutateKVValue(h.kv, "event_webhooks", key, &list, 0, func(exists bool) error {
		if !exists {
			return errWebhookListMissing
		}
		filtered := make([]EventWebhookConfig, 0, len(list.Webhooks))
		for _, wh := range list.Webhooks {
			if wh.ID != id {
				filtered = append(filtered, wh)
			}
		}
		list.Webhooks = filtered
		return nil
	})
	if errors.Is(err, errWebhookListMissing) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook configs")
		return
	}
	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventEventWebhookDeleted, "api",
			map[string]string{"id": id}))
	}
	w.WriteHeader(http.StatusNoContent)
}

// ──────────────────── webhook_replay.go ────────────────────
// WebhookReplayHandler re-triggers a webhook from its delivery log.
type WebhookReplayHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewWebhookReplayHandler(store core.Store, events *core.EventBus) *WebhookReplayHandler {
	return &WebhookReplayHandler{store: store, events: events}
}

// Replay handles POST /api/v1/apps/{id}/webhooks/{logId}/replay
func (h *WebhookReplayHandler) Replay(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	logID, ok := requirePathParam(w, r, "logId")
	if !ok {
		return
	}
	// Would look up the original webhook payload from webhook_logs table
	// and re-dispatch it through the build→deploy pipeline
	publishEventAsync(r.Context(), h.events, core.NewEvent(core.EventWebhookReceived, "api",
		core.WebhookEventData{WebhookID: logID, Provider: "replay"}))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":  app.ID,
		"log_id":  logID,
		"status":  "replaying",
		"message": "webhook delivery replayed — build pipeline triggered",
	})
}

// ──────────────────── webhook_logs.go ────────────────────
// WebhookLogHandler serves outbound webhook delivery history.
type WebhookLogHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewWebhookLogHandler(store core.Store, kv core.KVStorer) *WebhookLogHandler {
	return &WebhookLogHandler{store: store, kv: kv}
}

// WebhookDeliveryLog represents a single webhook delivery attempt.
// The shape mirrors webhooks.DeliveryLog so the persisted JSON the
// DeliveryTracker writes round-trips back to the API surface unchanged.
type WebhookDeliveryLog struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Status    string `json:"status"` // "sent" | "failed"
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
	TenantID  string `json:"tenant_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

const deliveryLogBucket = "webhook_delivery_log"

// List handles GET /api/v1/apps/{id}/webhooks/logs.
// Returns recent outbound webhook deliveries written by the
// webhooks.DeliveryTracker. The tracker doesn't yet record app
// affinity, so we return the full delivery log scoped to the
// authenticated tenant's admin view; per-app correlation will follow
// once outbound webhook configs carry an app_id label.
func (h *WebhookLogHandler) List(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	if h.kv == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	keys, err := h.kv.List(deliveryLogBucket)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	logs := make([]WebhookDeliveryLog, 0, len(keys))
	for _, k := range keys {
		var entry WebhookDeliveryLog
		if h.kv.Get(deliveryLogBucket, k, &entry) == nil {
			if entry.TenantID == app.TenantID {
				logs = append(logs, entry)
			}
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].Timestamp > logs[j].Timestamp })
	const maxLogs = 200
	if len(logs) > maxLogs {
		logs = logs[:maxLogs]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  logs,
		"total": len(logs),
	})
}

// ──────────────────── webhook_rotate.go ────────────────────
const webhookSecretsBucket = "webhooks"

// WebhookRotateHandler rotates webhook signing secrets.
type WebhookRotateHandler struct {
	store  core.Store
	events *core.EventBus
	kv     core.KVStorer
}

func NewWebhookRotateHandler(store core.Store, events *core.EventBus, kv core.KVStorer) *WebhookRotateHandler {
	return &WebhookRotateHandler{store: store, events: events, kv: kv}
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
	publishEventAsync(r.Context(), h.events, core.NewEvent("webhook.rotated", "api",
		map[string]string{"app_id": app.ID}))
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":     app.ID,
		"new_secret": newSecret,
		"message":    "Webhook secret rotated. Update your Git provider's webhook configuration.",
	})
}
func (h *WebhookRotateHandler) persistSecret(app *core.Application, secret string) error {
	if h.kv == nil {
		return errors.New("webhook secret store not configured")
	}
	var rec webhookSecretRecord
	if err := h.kv.Get(webhookSecretsBucket, app.ID, &rec); err != nil && !errors.Is(err, core.ErrKVNotFound) {
		return err
	}
	now := time.Now().UTC()
	if rec.ID == "" {
		rec.ID = app.ID
		rec.CreatedAt = now
	}
	rec.AppID = app.ID
	rec.SecretHash = hashSecret(secret) // SHA-256 hash — plaintext secret is never persisted
	rec.Status = "active"
	rec.UpdatedAt = now
	return h.kv.Set(webhookSecretsBucket, app.ID, rec, 0)
}

// ──────────────────── webhook_test_delivery.go ────────────────────
// WebhookTestDeliveryHandler sends a test webhook payload.
type WebhookTestDeliveryHandler struct {
	store  core.Store
	events *core.EventBus
	kv     core.KVStorer
}

func NewWebhookTestDeliveryHandler(store core.Store, events *core.EventBus, kv core.KVStorer) *WebhookTestDeliveryHandler {
	return &WebhookTestDeliveryHandler{store: store, events: events, kv: kv}
}

// webhookTestLog records test delivery results.
type webhookTestLog struct {
	AppID     string `json:"app_id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	PayloadID string `json:"payload_id"`
}

// TestDeliver handles POST /api/v1/apps/{id}/webhooks/test
// Sends a fake push event to the app's webhook endpoint.
func (h *WebhookTestDeliveryHandler) TestDeliver(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	deliveryID := core.GenerateID()
	testPayload := map[string]any{
		"event":     "push",
		"ref":       "refs/heads/main",
		"test":      true,
		"timestamp": time.Now().Format(time.RFC3339),
		"sender":    "deploymonster-test",
		"repository": map[string]string{
			"full_name": "test/repo",
		},
		"head_commit": map[string]string{
			"id":      "test-" + core.ShortID(deliveryID, 8),
			"message": "Test webhook delivery from DeployMonster",
		},
	}
	payload, _ := json.Marshal(testPayload)
	// Emit event so the outbound webhook system picks it up
	publishEvent(r.Context(), h.events, core.NewEvent("webhook.test."+appID, "api", map[string]any{
		"app_id":      appID,
		"delivery_id": deliveryID,
		"payload":     string(payload),
	}))
	// Log the test delivery
	log := webhookTestLog{
		AppID:     appID,
		Status:    "delivered",
		Timestamp: time.Now().Format(time.RFC3339),
		PayloadID: deliveryID,
	}
	if err := h.kv.Set("webhook_test_logs", deliveryID, log, 86400); err != nil {
		slog.Error("failed to persist webhook test log", "delivery_id", deliveryID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":      appID,
		"delivery_id": deliveryID,
		"status":      "delivered",
		"payload":     testPayload,
	})
}
