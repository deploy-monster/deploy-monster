package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// =============================================================================
// DeliveryTracker.record — nil bolt path (delivery_log.go:73)
// =============================================================================

// TestDeliveryTracker_Record_NilBolt covers the nil-bolt early return in
// record(). Create a tracker with bolt=nil, then call Start(), which returns
// immediately because bolt is nil. Then call record directly (it's
// accessible because we're in the same package).
func TestDeliveryTracker_Record_NilBolt(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(nil, events)
	// record with nil bolt should return nil (no-op)
	err := dt.record(DeliveryLog{
		ID:     "test-id",
		URL:    "https://example.com/hook",
		Status: "sent",
	})
	if err != nil {
		t.Errorf("record with nil bolt should return nil, got: %v", err)
	}
}

// =============================================================================
// DeliveryTracker.Start — EventOutboundFailed with wrong data type
// (delivery_log.go:57-59)
// =============================================================================

// TestDeliveryTracker_Start_FailedWrongDataType covers the !ok path in the
// EventOutboundFailed subscription handler.
func TestDeliveryTracker_Start_FailedWrongDataType(t *testing.T) {
	bolt := &mockBoltStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(bolt, events)
	dt.Start()

	// Emit EventOutboundFailed with wrong data type — handler should return
	// nil without recording (the !ok path).
	events.EmitWithTenant(context.Background(), core.EventOutboundFailed, "webhook", "t1", "u1", "not-notification-data")

	// Drain async handlers
	events.Drain()

	if bolt.wasCalled() {
		t.Error("record should NOT have been called for wrong data type on failed event")
	}
}

// =============================================================================
// deliveryDedupKey — empty body path (receiver.go:56-58)
// =============================================================================

// TestDeliveryDedupKey_EmptyBody covers the len(body) == 0 early return.
func TestDeliveryDedupKey_EmptyBody(t *testing.T) {
	r := &http.Request{Header: http.Header{}}
	// No provider-delivery headers and empty body → should return ""
	key := deliveryDedupKey("wh-1", nil, r)
	if key != "" {
		t.Errorf("expected empty key for no headers + nil body, got %q", key)
	}

	key = deliveryDedupKey("wh-2", []byte{}, r)
	if key != "" {
		t.Errorf("expected empty key for no headers + empty body, got %q", key)
	}
}

// =============================================================================
// HandleWebhook — dedup Set error path (receiver.go:132-135)
// =============================================================================

// boltSetFailing returns an error on Set for the dedup bucket.
type boltSetFailing struct {
	secret string
	setErr error
}

func (m *boltSetFailing) Set(_, _ string, _ any, _ int64) error { return m.setErr }
func (m *boltSetFailing) BatchSet(_ []core.BoltBatchItem) error  { return nil }
func (m *boltSetFailing) Get(_, _ string, _ any) error {
	// Return ErrKVNotFound so dedup proceeds to Set
	return core.ErrKVNotFound
}
func (m *boltSetFailing) Delete(_, _ string) error    { return nil }
func (m *boltSetFailing) List(_ string) ([]string, error) { return nil, nil }
func (m *boltSetFailing) Close() error                { return nil }
func (m *boltSetFailing) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, core.ErrKVNotFound
}
func (m *boltSetFailing) GetWebhookSecret(_ string) (string, error) { return m.secret, nil }

func TestHandleWebhook_DedupSetError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	bolt := &boltSetFailing{secret: "s", setErr: fmt.Errorf("dedup write failed")}
	recv := NewReceiver(nil, bolt, events, logger)

	body := `{"ref":"refs/heads/main"}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-dedup-fail", strings.NewReader(body))
	req.SetPathValue("webhookID", "wh-dedup-fail")
	rr := httptest.NewRecorder()

	recv.HandleWebhook(rr, req)

	// The handler should still return 200 even if the dedup Set fails;
	// the error is just logged as a warning.
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 despite dedup set error, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// parseBitbucket — flat envelope EventType fallback (receiver.go:297-299)
// =============================================================================

// TestParseBitbucket_FlatEnvelopeEventTypeFallback covers the
// p.EventType == "" && flat.EventType != "" branch in parseBitbucket's flat
// envelope fallback.
func TestParseBitbucket_FlatEnvelopeEventTypeFallback(t *testing.T) {
	// Flat envelope JSON without native push structure and with event_type
	body := []byte(`{"event_type":"repo:push","branch":"main"}`)
	r := &http.Request{Header: http.Header{}}
	// X-Event-Key not set → deliveryDedupKey's p.EventType would be ""
	// unless the flat envelope provides it

	p, err := parseBitbucket(body, r)
	if err != nil {
		t.Fatalf("parseBitbucket: %v", err)
	}
	if p.EventType != "repo:push" {
		t.Errorf("event_type = %q, want repo:push (from flat envelope fallback)", p.EventType)
	}
	if p.Branch != "main" {
		t.Errorf("branch = %q, want main", p.Branch)
	}
}

// =============================================================================
// deliveryDedupKey — body hash key (receiver.go:59-60) — bonus: tests the
// SHA-256 fallback when no provider header is present.
// =============================================================================

func TestDeliveryDedupKey_BodyHashFallback(t *testing.T) {
	r := &http.Request{Header: http.Header{}}
	// No provider delivery headers → falls back to SHA-256 of body
	key := deliveryDedupKey("wh-1", []byte(`{"ref":"main"}`), r)
	if key == "" {
		t.Fatal("expected non-empty key for body hash fallback")
	}
	if !strings.HasPrefix(key, "wh-1:") {
		t.Errorf("key should start with webhookID: prefix, got %q", key)
	}
	// Same input should produce the same key
	key2 := deliveryDedupKey("wh-1", []byte(`{"ref":"main"}`), r)
	if key != key2 {
		t.Errorf("same input should produce same key: %q vs %q", key, key2)
	}
	// Different input should produce different key
	key3 := deliveryDedupKey("wh-1", []byte(`{"ref":"other"}`), r)
	if key == key3 {
		t.Error("different body should produce different key")
	}
}

// =============================================================================
// HandleWebhook — dedup path when delivery already seen
// (receiver.go:124-131) - supplement to existing TestHandleWebhook_RejectsReplayedDelivery
// =============================================================================

// TestHandleWebhook_DedupSentinel verifies the duplicate response shape
// even when there is no EventBus subscriber to count events.
func TestHandleWebhook_DedupResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)

	bolt := &statefulBolt{secret: "s"}
	recv := NewReceiver(nil, bolt, events, logger)

	send := func() *httptest.ResponseRecorder {
		body := `{"ping": true}`
		req := httptest.NewRequest("POST", "/hooks/v1/wh-dedup2", strings.NewReader(body))
		req.SetPathValue("webhookID", "wh-dedup2")
		req.Header.Set("X-Request-Id", "dup-delivery-xyz")
		rr := httptest.NewRecorder()
		recv.HandleWebhook(rr, req)
		return rr
	}

	first := send()
	if first.Code != http.StatusOK {
		t.Fatalf("first: expected 200, got %d", first.Code)
	}

	second := send()
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate: expected 200, got %d", second.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(second.Body).Decode(&resp)
	if resp["status"] != "duplicate" {
		t.Errorf("duplicate status = %q, want %q", resp["status"], "duplicate")
	}
}

// =============================================================================
// DeliveryTracker — concurrent use of mockBoltStoreDelivery (bonus: race check)
// =============================================================================

func TestDeliveryTracker_Concurrent(t *testing.T) {
	bolt := &mockBoltStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(bolt, events)
	dt.Start()

	// Emit events concurrently
	for i := 0; i < 10; i++ {
		go func() {
			events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", core.NotificationEventData{
				Recipient: fmt.Sprintf("https://example%d.com/hook", i),
			})
		}()
	}

	events.Drain()
}
