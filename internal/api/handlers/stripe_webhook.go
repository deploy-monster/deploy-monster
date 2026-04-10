package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/billing"
)

// maxStripeWebhookBody bounds the webhook payload we will accept. Stripe's own
// documented limit is 256 KB; we double it to stay robust against large event
// objects while still rejecting absurdly oversized bodies.
const maxStripeWebhookBody = 512 << 10 // 512 KB

// StripeWebhookHandler adapts HTTP webhook requests into calls against the
// billing.StripeEventHandler. The handler is intentionally thin: it only reads
// the body and signature header, then delegates dispatch to the billing
// package so the business logic stays out of the HTTP layer.
type StripeWebhookHandler struct {
	events *billing.StripeEventHandler
	logger *slog.Logger
}

// NewStripeWebhookHandler constructs a webhook handler. `events` must be
// non-nil for the handler to actually process events; when it is nil, every
// request is rejected with 503 so operators notice the misconfiguration.
func NewStripeWebhookHandler(events *billing.StripeEventHandler, logger *slog.Logger) *StripeWebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StripeWebhookHandler{events: events, logger: logger}
}

// ServeHTTP implements the POST /api/v1/webhooks/stripe route. Stripe
// authenticates webhooks via the `Stripe-Signature` header, so we do not apply
// our usual bearer-auth middleware on this endpoint.
//
// Status codes:
//
//	200 — event accepted (or ignored type acknowledged)
//	400 — malformed body / bad or missing signature
//	413 — body exceeded the configured limit
//	500 — transient error; Stripe should retry
//	503 — billing not configured
func (h *StripeWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.events == nil {
		writeError(w, http.StatusServiceUnavailable, "stripe billing not configured")
		return
	}

	signature := r.Header.Get("Stripe-Signature")
	if signature == "" {
		writeError(w, http.StatusBadRequest, "missing Stripe-Signature header")
		return
	}

	// Wrap the body with MaxBytesReader so the client sees a clean 413 if they
	// send more than the limit (io.ReadAll below would otherwise return a
	// generic error).
	r.Body = http.MaxBytesReader(w, r.Body, maxStripeWebhookBody)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	if err := h.events.Handle(r.Context(), payload, signature); err != nil {
		if errors.Is(err, billing.ErrStripeInvalidSignature) {
			// Log at info level — this is expected during credential rotation
			// and we do not want to fill error logs with noise.
			h.logger.Info("stripe webhook: signature rejected",
				"remote_addr", r.RemoteAddr)
			writeError(w, http.StatusBadRequest, "invalid signature")
			return
		}
		// Any other error is a transient backend problem — return 500 so
		// Stripe retries delivery.
		h.logger.Warn("stripe webhook: handler error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"received": true})
}
