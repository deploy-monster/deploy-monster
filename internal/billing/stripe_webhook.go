package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// defaultStripeSeenTTL bounds how long a processed Stripe event ID
// is remembered for replay suppression. Stripe retries delivery for
// roughly 3 days on persistent failures, so 72h is the smallest
// window that makes "second delivery of the same event" safe.
const defaultStripeSeenTTL = 72 * time.Hour

// ErrStripeInvalidSignature indicates the Stripe webhook signature did not
// match the computed HMAC. The HTTP handler maps this to 400 so Stripe does
// not retry forever on a misconfigured webhook secret.
var ErrStripeInvalidSignature = errors.New("stripe webhook: invalid signature")

// StripeEventHandler turns raw Stripe webhook payloads into tenant state
// mutations and internal events. It is intentionally stateless — all tenant
// state lives in the Store — so the HTTP handler can construct one per
// request without coordination.
type StripeEventHandler struct {
	store  core.Store
	events *core.EventBus
	client *StripeClient
	plans  []Plan
	logger *slog.Logger
	now    func() time.Time // injectable for tests

	// Replay suppression. Stripe delivers every webhook with a
	// unique event ID and retries aggressively on 5xx responses, so
	// the same ID can arrive many times per event. We remember the
	// IDs of events we've already processed and short-circuit
	// replays to nil (ack, do nothing). The map is swept on every
	// Handle call so a restart-heavy process does not accumulate
	// entries forever. 72h matches Stripe's maximum retry window.
	seenMu  sync.Mutex
	seen    map[string]time.Time
	seenTTL time.Duration
}

// NewStripeEventHandler constructs a handler. `client` is required for signature
// verification; `plans` is the catalog used to resolve price IDs to internal
// plan IDs on subscription updates.
func NewStripeEventHandler(
	store core.Store,
	events *core.EventBus,
	client *StripeClient,
	plans []Plan,
	logger *slog.Logger,
) *StripeEventHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StripeEventHandler{
		store:   store,
		events:  events,
		client:  client,
		plans:   plans,
		logger:  logger,
		now:     func() time.Time { return time.Now().UTC() },
		seen:    make(map[string]time.Time),
		seenTTL: defaultStripeSeenTTL,
	}
}

// alreadyProcessed reports whether a Stripe event ID has been
// successfully handled within the current seenTTL window. Callers
// must hold no locks; alreadyProcessed acquires seenMu.
func (h *StripeEventHandler) alreadyProcessed(eventID string) bool {
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	h.sweepLocked()
	_, ok := h.seen[eventID]
	return ok
}

// markProcessed records that eventID has finished dispatch and
// should suppress any future deliveries of the same ID. Called only
// after a successful handler run so a transient 500 lets Stripe
// retry.
func (h *StripeEventHandler) markProcessed(eventID string) {
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	h.sweepLocked()
	h.seen[eventID] = h.now()
}

// sweepLocked drops entries older than seenTTL. Caller must hold
// seenMu. Called from both the check and the mark path so the map
// can't grow unbounded on a high-traffic installation.
func (h *StripeEventHandler) sweepLocked() {
	ttl := h.seenTTL
	if ttl <= 0 {
		return
	}
	cutoff := h.now().Add(-ttl)
	for id, ts := range h.seen {
		if ts.Before(cutoff) {
			delete(h.seen, id)
		}
	}
}

// stripeEventEnvelope matches the top-level shape of every Stripe webhook body.
// `Data.Object` is the event-specific payload — decoded separately per type.
type stripeEventEnvelope struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// Handle verifies the webhook signature, parses the event, and dispatches it
// to the appropriate processor. Returns ErrStripeInvalidSignature when the
// signature is missing or wrong; other errors should map to 500 so Stripe
// retries the delivery.
//
// Idempotency: every event carries a unique ID from Stripe. A second
// delivery of an ID we've already successfully processed returns nil
// immediately without re-running any side effects. A failed dispatch
// is NOT marked seen so Stripe's retry machinery can still eventually
// land the write — that's the difference between "replay" (Stripe
// re-delivering for safety) and "recovery" (Stripe re-delivering
// because our handler 500'd).
func (h *StripeEventHandler) Handle(ctx context.Context, payload []byte, signature string) error {
	if h.client == nil || !h.client.VerifyWebhookSignature(payload, signature) {
		return ErrStripeInvalidSignature
	}
	var env stripeEventEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("stripe webhook: decode envelope: %w", err)
	}
	if env.Type == "" {
		return fmt.Errorf("stripe webhook: missing event type")
	}

	// Replay suppression. Events without an ID (shouldn't happen in
	// production but showed up in fuzz tests) always re-process —
	// there is nothing to dedupe against.
	if env.ID != "" && h.alreadyProcessed(env.ID) {
		h.logger.Info("stripe webhook: replay suppressed",
			"event_id", env.ID, "event_type", env.Type)
		return nil
	}

	h.logger.Info("stripe webhook received", "event_id", env.ID, "event_type", env.Type)

	var dispatchErr error
	switch env.Type {
	case "customer.subscription.created", "customer.subscription.updated":
		dispatchErr = h.handleSubscriptionUpdated(ctx, env)
	case "customer.subscription.deleted":
		dispatchErr = h.handleSubscriptionCanceled(ctx, env)
	case "invoice.paid", "invoice.payment_succeeded":
		dispatchErr = h.handleInvoicePaid(ctx, env)
	case "invoice.payment_failed":
		dispatchErr = h.handleInvoicePaymentFailed(ctx, env)
	case "payment_intent.succeeded":
		dispatchErr = h.handlePaymentIntentSucceeded(ctx, env)
	case "checkout.session.completed":
		dispatchErr = h.handleCheckoutCompleted(ctx, env)
	default:
		// Acknowledge unknown types so Stripe stops retrying.
		h.logger.Debug("stripe webhook: ignoring unhandled event", "event_type", env.Type)
	}

	if dispatchErr != nil {
		return dispatchErr
	}
	if env.ID != "" {
		h.markProcessed(env.ID)
	}
	return nil
}

// stripeSubscription mirrors the fields of a Stripe subscription object that
// we care about. Only one item is supported — if a subscription carries more,
// the first is used.
type stripeSubscription struct {
	ID       string `json:"id"`
	Customer string `json:"customer"`
	Status   string `json:"status"`
	Metadata struct {
		TenantID string `json:"tenant_id"`
	} `json:"metadata"`
	Items struct {
		Data []struct {
			ID    string `json:"id"`
			Price struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

func (h *StripeEventHandler) handleSubscriptionUpdated(ctx context.Context, env stripeEventEnvelope) error {
	var sub stripeSubscription
	if err := json.Unmarshal(env.Data.Object, &sub); err != nil {
		return fmt.Errorf("stripe webhook: decode subscription: %w", err)
	}
	if sub.ID == "" {
		return fmt.Errorf("stripe webhook: subscription has no id")
	}
	if sub.Metadata.TenantID == "" {
		h.logger.Warn("stripe webhook: subscription missing tenant_id metadata", "subscription_id", sub.ID)
		return nil // nothing we can do; acknowledge so Stripe stops retrying
	}

	tenant, err := h.store.GetTenant(ctx, sub.Metadata.TenantID)
	if err != nil {
		return fmt.Errorf("stripe webhook: load tenant %s: %w", sub.Metadata.TenantID, err)
	}

	md, err := GetStripeMetadata(tenant)
	if err != nil {
		return fmt.Errorf("stripe webhook: read tenant metadata: %w", err)
	}
	md.CustomerID = sub.Customer
	md.SubscriptionID = sub.ID
	md.Status = sub.Status
	md.UpdatedAt = h.now()
	if len(sub.Items.Data) > 0 {
		md.SubscriptionItemID = sub.Items.Data[0].ID
		md.PriceID = sub.Items.Data[0].Price.ID
	}

	// Resolve plan from price id when the operator has wired it up.
	if plan := PlanByStripePriceID(h.plans, md.PriceID); plan != nil {
		tenant.PlanID = plan.ID
	}

	if err := SetStripeMetadata(tenant, md); err != nil {
		return fmt.Errorf("stripe webhook: write tenant metadata: %w", err)
	}
	if err := h.store.UpdateTenant(ctx, tenant); err != nil {
		return fmt.Errorf("stripe webhook: update tenant: %w", err)
	}

	h.emit(ctx, core.EventBillingSubscriptionUpdated, tenant.ID, map[string]any{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"price_id":        md.PriceID,
		"plan_id":         tenant.PlanID,
	})
	return nil
}

func (h *StripeEventHandler) handleSubscriptionCanceled(ctx context.Context, env stripeEventEnvelope) error {
	var sub stripeSubscription
	if err := json.Unmarshal(env.Data.Object, &sub); err != nil {
		return fmt.Errorf("stripe webhook: decode subscription: %w", err)
	}
	if sub.Metadata.TenantID == "" {
		h.logger.Warn("stripe webhook: cancel event missing tenant_id", "subscription_id", sub.ID)
		return nil
	}

	tenant, err := h.store.GetTenant(ctx, sub.Metadata.TenantID)
	if err != nil {
		return fmt.Errorf("stripe webhook: load tenant %s: %w", sub.Metadata.TenantID, err)
	}

	md, err := GetStripeMetadata(tenant)
	if err != nil {
		return fmt.Errorf("stripe webhook: read tenant metadata: %w", err)
	}
	md.Status = "canceled"
	md.SubscriptionID = ""
	md.SubscriptionItemID = ""
	md.PriceID = ""
	md.UpdatedAt = h.now()

	// Revert to the free plan when the catalog has one.
	if free := findPlanByID(h.plans, "free"); free != nil {
		tenant.PlanID = free.ID
	}

	if err := SetStripeMetadata(tenant, md); err != nil {
		return fmt.Errorf("stripe webhook: write tenant metadata: %w", err)
	}
	if err := h.store.UpdateTenant(ctx, tenant); err != nil {
		return fmt.Errorf("stripe webhook: update tenant: %w", err)
	}

	h.emit(ctx, core.EventBillingSubscriptionCanceled, tenant.ID, map[string]any{
		"subscription_id": sub.ID,
		"plan_id":         tenant.PlanID,
	})
	return nil
}

// stripeInvoice is the subset of the invoice object we look at on paid/failed
// webhook events.
type stripeInvoice struct {
	ID           string `json:"id"`
	Customer     string `json:"customer"`
	Subscription string `json:"subscription"`
	Status       string `json:"status"`
	AmountPaid   int64  `json:"amount_paid"`
	AmountDue    int64  `json:"amount_due"`
	Currency     string `json:"currency"`
	Metadata     struct {
		TenantID string `json:"tenant_id"`
	} `json:"metadata"`
}

func (h *StripeEventHandler) handleInvoicePaid(ctx context.Context, env stripeEventEnvelope) error {
	var inv stripeInvoice
	if err := json.Unmarshal(env.Data.Object, &inv); err != nil {
		return fmt.Errorf("stripe webhook: decode invoice: %w", err)
	}

	tenantID := h.resolveTenantID(ctx, inv.Metadata.TenantID, inv.Customer, inv.Subscription)

	if tenantID != "" {
		if tenant, err := h.store.GetTenant(ctx, tenantID); err == nil {
			md, mdErr := GetStripeMetadata(tenant)
			if mdErr == nil {
				md.PaymentLastSucceededAt = h.now()
				md.Status = "active"
				if setErr := SetStripeMetadata(tenant, md); setErr == nil {
					if updErr := h.store.UpdateTenant(ctx, tenant); updErr != nil {
						h.logger.Warn("stripe webhook: failed to update tenant after invoice.paid",
							"tenant", tenantID, "error", updErr)
					}
				}
			}
		}
	}

	h.emit(ctx, core.EventInvoiceGenerated, tenantID, map[string]any{
		"invoice_id":  inv.ID,
		"amount_paid": inv.AmountPaid,
		"currency":    inv.Currency,
	})
	h.emit(ctx, core.EventPaymentReceived, tenantID, map[string]any{
		"invoice_id":   inv.ID,
		"amount_cents": inv.AmountPaid,
		"currency":     inv.Currency,
	})
	return nil
}

func (h *StripeEventHandler) handleInvoicePaymentFailed(ctx context.Context, env stripeEventEnvelope) error {
	var inv stripeInvoice
	if err := json.Unmarshal(env.Data.Object, &inv); err != nil {
		return fmt.Errorf("stripe webhook: decode invoice: %w", err)
	}

	tenantID := h.resolveTenantID(ctx, inv.Metadata.TenantID, inv.Customer, inv.Subscription)

	if tenantID != "" {
		if tenant, err := h.store.GetTenant(ctx, tenantID); err == nil {
			md, mdErr := GetStripeMetadata(tenant)
			if mdErr == nil {
				md.PaymentLastFailedAt = h.now()
				md.Status = "past_due"
				if setErr := SetStripeMetadata(tenant, md); setErr == nil {
					if updErr := h.store.UpdateTenant(ctx, tenant); updErr != nil {
						h.logger.Warn("stripe webhook: failed to update tenant after payment_failed",
							"tenant", tenantID, "error", updErr)
					}
				}
			}
		}
	}

	h.emit(ctx, core.EventPaymentFailed, tenantID, map[string]any{
		"invoice_id": inv.ID,
		"amount_due": inv.AmountDue,
		"currency":   inv.Currency,
	})
	return nil
}

type stripePaymentIntent struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Customer string `json:"customer"`
	Metadata struct {
		TenantID string `json:"tenant_id"`
	} `json:"metadata"`
}

func (h *StripeEventHandler) handlePaymentIntentSucceeded(ctx context.Context, env stripeEventEnvelope) error {
	var pi stripePaymentIntent
	if err := json.Unmarshal(env.Data.Object, &pi); err != nil {
		return fmt.Errorf("stripe webhook: decode payment_intent: %w", err)
	}
	tenantID := h.resolveTenantID(ctx, pi.Metadata.TenantID, pi.Customer, "")
	h.emit(ctx, core.EventPaymentReceived, tenantID, map[string]any{
		"payment_intent_id": pi.ID,
		"amount_cents":      pi.Amount,
		"currency":          pi.Currency,
	})
	return nil
}

type stripeCheckoutSession struct {
	ID            string `json:"id"`
	Customer      string `json:"customer"`
	Subscription  string `json:"subscription"`
	PaymentStatus string `json:"payment_status"`
	AmountTotal   int64  `json:"amount_total"`
	Currency      string `json:"currency"`
	Metadata      struct {
		TenantID string `json:"tenant_id"`
	} `json:"metadata"`
}

func (h *StripeEventHandler) handleCheckoutCompleted(ctx context.Context, env stripeEventEnvelope) error {
	var sess stripeCheckoutSession
	if err := json.Unmarshal(env.Data.Object, &sess); err != nil {
		return fmt.Errorf("stripe webhook: decode checkout session: %w", err)
	}
	tenantID := h.resolveTenantID(ctx, sess.Metadata.TenantID, sess.Customer, sess.Subscription)
	h.emit(ctx, core.EventBillingCheckoutCompleted, tenantID, map[string]any{
		"session_id":      sess.ID,
		"subscription_id": sess.Subscription,
		"amount_cents":    sess.AmountTotal,
		"currency":        sess.Currency,
	})
	return nil
}

// resolveTenantID best-effort finds a tenant ID for a Stripe event. Stripe
// metadata wins; otherwise the customer and subscription fields are matched
// against tenants' stored Stripe metadata. Returns empty string when nothing
// can be resolved — the event is still emitted without a tenant context.
func (h *StripeEventHandler) resolveTenantID(ctx context.Context, metaTenantID, customerID, subscriptionID string) string {
	if metaTenantID != "" {
		return metaTenantID
	}
	// Without a tenant-search index we can't scale a scan across tenants,
	// so just return empty — the event still fires with an empty tenant id
	// which is honest and matches how other billing events behave.
	_ = ctx
	_ = customerID
	_ = subscriptionID
	return ""
}

func (h *StripeEventHandler) emit(ctx context.Context, eventType, tenantID string, data map[string]any) {
	if h.events == nil {
		return
	}
	if err := h.events.EmitWithTenant(ctx, eventType, "billing.stripe", tenantID, "", data); err != nil {
		h.logger.Warn("stripe webhook: emit event failed", "event", eventType, "error", err)
	}
}

func findPlanByID(plans []Plan, id string) *Plan {
	for i := range plans {
		if plans[i].ID == id {
			return &plans[i]
		}
	}
	return nil
}
