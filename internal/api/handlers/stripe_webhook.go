package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/billing"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// maxStripeWebhookBody bounds the webhook payload we will accept. Stripe's own
// documented limit is 256 KB; we double it to stay robust against large event
// objects while still rejecting absurdly oversized bodies.
const maxStripeWebhookBody = 512 << 10 // 512 KB

// stripeWebhookRateLimit is the max webhook deliveries per minute per IP.
// Stripe itself sends at most a few per minute even at high volume.
// 30/min allows headroom while blocking flood attacks.
const stripeWebhookRateLimit = 30
const stripeWebhookRateWindow = time.Minute

// stripeRateLimitEntry tracks deliveries for a single IP.
type stripeRateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

// StripeWebhookHandler adapts HTTP webhook requests into calls against the
// billing.StripeEventHandler. The handler is intentionally thin: it only reads
// the body and signature header, then delegates dispatch to the billing
// package so the business logic stays out of the HTTP layer.
type StripeWebhookHandler struct {
	events *billing.StripeEventHandler
	bolt   core.BoltStorer
	logger *slog.Logger
	// In-memory rate limit state — independent per process (ephemeral, but
	// Stripe itself rate-limits at the source so this is a second layer).
	mu       sync.Mutex
	ipLimits map[string]*stripeRateLimitEntry
}

// NewStripeWebhookHandler constructs a webhook handler. `events` must be
// non-nil for the handler to actually process events; when it is nil, every
// request is rejected with 503 so operators notice the misconfiguration.
func NewStripeWebhookHandler(events *billing.StripeEventHandler, bolt core.BoltStorer, logger *slog.Logger) *StripeWebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StripeWebhookHandler{
		events:   events,
		bolt:     bolt,
		logger:   logger,
		ipLimits: make(map[string]*stripeRateLimitEntry),
	}
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
//	429 — rate limit exceeded
//	500 — transient error; Stripe should retry
//	503 — billing not configured
func (h *StripeWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.events == nil {
		writeError(w, http.StatusServiceUnavailable, "stripe billing not configured")
		return
	}

	ip := stripPort(r.RemoteAddr)
	h.mu.Lock()
	entry, ok := h.ipLimits[ip]
	now := time.Now()
	if !ok || now.After(entry.ResetAt) {
		entry = &stripeRateLimitEntry{
			Count:   1,
			ResetAt: now.Add(stripeWebhookRateWindow),
		}
		h.ipLimits[ip] = entry
		h.mu.Unlock()
	} else if entry.Count >= stripeWebhookRateLimit {
		retryAfter := int(entry.ResetAt.Sub(now).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		h.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	} else {
		entry.Count++
		h.mu.Unlock()
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

// stripPort removes the :port suffix from RemoteAddr (e.g. "1.2.3.4:8080" -> "1.2.3.4").
func stripPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		if !strings.Contains(addr, "]") && strings.Count(addr, ":") == 1 {
			return addr[:i]
		}
	}
	return addr
}
