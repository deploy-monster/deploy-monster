package billing

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestStripeWebhook_Replay_SameEventIDIsNoop is the Phase 3.2.5
// headline: Stripe retries on 5xx and occasionally re-sends events
// for other reasons (dashboard "resend", network partitions, etc.).
// Without event-ID deduping, a re-delivered subscription.updated
// would re-run UpdateTenant — harmless on its own — but a
// re-delivered invoice.paid would emit a second PaymentReceived
// event to downstream listeners, which would double-count revenue
// in any billing dashboard.
//
// This test delivers the SAME event ID twice and asserts:
//
//  1. First call mutates the store (updates == 1).
//  2. Second call is a no-op (updates stays at 1).
//  3. EventBillingSubscriptionUpdated fires exactly once even though
//     Handle was called twice.
func TestStripeWebhook_Replay_SameEventIDIsNoop(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-replay", PlanID: "free"}
	store := newWebhookStore(tenant)
	plans := []Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	h, secret := newTestHandler(t, store, plans)

	// Count billing events so we can assert "fired exactly once".
	var emitted atomic.Int64
	h.events.Subscribe(core.EventBillingSubscriptionUpdated, func(_ context.Context, _ core.Event) error {
		emitted.Add(1)
		return nil
	})

	payload := []byte(`{
		"id":"evt_replay_1",
		"type":"customer.subscription.updated",
		"data":{"object":{
			"id":"sub_replay",
			"customer":"cus_replay",
			"status":"active",
			"metadata":{"tenant_id":"tenant-replay"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}
		}}
	}`)
	sig := signPayload(t, secret, payload)

	// First delivery — normal processing.
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if store.updates != 1 {
		t.Fatalf("after first delivery updates = %d, want 1", store.updates)
	}
	if got := store.tenants["tenant-replay"].PlanID; got != "pro" {
		t.Fatalf("after first delivery PlanID = %q, want pro", got)
	}

	// Second delivery of the SAME event ID — must be a no-op.
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("second Handle (replay): %v", err)
	}
	if store.updates != 1 {
		t.Errorf("after replay updates = %d, want 1 (replay should not re-mutate store)", store.updates)
	}

	// Give the event bus a moment to drain any async subscribers
	// (Subscribe is sync but defensive — Drain is a cheap wait).
	h.events.Drain()
	if got := emitted.Load(); got != 1 {
		t.Errorf("EventBillingSubscriptionUpdated fired %d times, want 1", got)
	}
}

// TestStripeWebhook_Replay_FailedHandlerAllowsRetry is the
// companion to the happy-path replay test: if dispatch FAILS (e.g.
// the database is down), the event ID must NOT be marked seen so
// Stripe's retry delivers it again and we successfully land the
// write on the second try. This is the "Stripe replay" vs "Stripe
// retry" distinction — replays are no-ops, retries after failure
// must still work.
func TestStripeWebhook_Replay_FailedHandlerAllowsRetry(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-retry", PlanID: "free"}
	store := newWebhookStore(tenant)
	store.updateErr = errors.New("db is down")
	plans := []Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	h, secret := newTestHandler(t, store, plans)

	payload := []byte(`{
		"id":"evt_retry_1",
		"type":"customer.subscription.updated",
		"data":{"object":{
			"id":"sub_retry",
			"customer":"cus_retry",
			"status":"active",
			"metadata":{"tenant_id":"tenant-retry"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}
		}}
	}`)
	sig := signPayload(t, secret, payload)

	// First delivery fails because the store is wedged. Handle
	// should return an error (so the HTTP layer returns 500 and
	// Stripe retries).
	if err := h.Handle(context.Background(), payload, sig); err == nil {
		t.Fatal("first Handle: expected error on DB failure, got nil")
	}
	if h.alreadyProcessed("evt_retry_1") {
		t.Fatal("alreadyProcessed=true after failed dispatch — retries would be suppressed")
	}

	// Recover the store and retry. Must succeed AND take effect.
	store.updateErr = nil
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("retry Handle: %v", err)
	}
	if store.updates != 1 {
		t.Errorf("after retry updates = %d, want 1", store.updates)
	}
	if got := store.tenants["tenant-retry"].PlanID; got != "pro" {
		t.Errorf("after retry PlanID = %q, want pro", got)
	}

	// Third delivery (replay of the now-successful event) must be a
	// no-op.
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("replay Handle: %v", err)
	}
	if store.updates != 1 {
		t.Errorf("after replay-post-retry updates = %d, want 1", store.updates)
	}
}

// TestStripeWebhook_Replay_DifferentEventIDsProcessSeparately
// verifies the dedup is keyed on event ID, not on subscription ID
// or type. Stripe sometimes sends multiple events for the same
// subscription (status flip → update), and each event has its own
// ID — they must NOT cancel each other out.
func TestStripeWebhook_Replay_DifferentEventIDsProcessSeparately(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-separate", PlanID: "free"}
	store := newWebhookStore(tenant)
	plans := []Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	h, secret := newTestHandler(t, store, plans)

	payload1 := []byte(`{
		"id":"evt_distinct_A",
		"type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","customer":"cus_1","status":"active",
			"metadata":{"tenant_id":"tenant-separate"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}}}
	}`)
	payload2 := []byte(`{
		"id":"evt_distinct_B",
		"type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","customer":"cus_1","status":"active",
			"metadata":{"tenant_id":"tenant-separate"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}}}
	}`)

	if err := h.Handle(context.Background(), payload1, signPayload(t, secret, payload1)); err != nil {
		t.Fatalf("Handle A: %v", err)
	}
	if err := h.Handle(context.Background(), payload2, signPayload(t, secret, payload2)); err != nil {
		t.Fatalf("Handle B: %v", err)
	}

	// Both events had distinct IDs so both should have processed,
	// even though the payload content is otherwise identical.
	if store.updates != 2 {
		t.Errorf("distinct event IDs updates = %d, want 2", store.updates)
	}
}

// TestStripeWebhook_Replay_TTLSweepExpiresOldEntries guards the
// memory-bound invariant: the seen map must not grow forever. We
// set a tiny TTL, process an event, advance the injected clock
// past the TTL, then verify alreadyProcessed returns false (so a
// very-late replay would be re-processed — the TTL window is
// specifically sized to Stripe's retry window so this is safe).
func TestStripeWebhook_Replay_TTLSweepExpiresOldEntries(t *testing.T) {
	store := newWebhookStore(&core.Tenant{ID: "tenant-ttl", PlanID: "free"})
	plans := []Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	h, secret := newTestHandler(t, store, plans)

	// Shrink the TTL to something the test can actually span, and
	// replace the clock with a controllable fake. Both fields are
	// unexported so the test lives in the same package.
	h.seenTTL = 50 * time.Millisecond
	fakeNow := time.Now()
	h.now = func() time.Time { return fakeNow }

	payload := []byte(`{
		"id":"evt_ttl_1",
		"type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","customer":"cus_1","status":"active",
			"metadata":{"tenant_id":"tenant-ttl"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !h.alreadyProcessed("evt_ttl_1") {
		t.Fatal("alreadyProcessed=false immediately after mark")
	}

	// Advance the fake clock past the TTL and trigger a sweep via
	// another alreadyProcessed call.
	fakeNow = fakeNow.Add(time.Second)
	if h.alreadyProcessed("evt_ttl_1") {
		t.Error("alreadyProcessed=true after TTL expiry — sweep should have dropped the entry")
	}

	// Now the same event ID should process again (disaster recovery
	// scenario: a 4-day-old Stripe retry lands after our process
	// lost its in-memory state).
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("late-retry Handle: %v", err)
	}
	if store.updates != 2 {
		t.Errorf("after TTL-expired replay updates = %d, want 2", store.updates)
	}
}
