package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// P1-14: generic-path signature hardening. Signing is opt-in — a generic
// webhook with no signature header keeps working (URL-ID bearer), but when a
// signature header IS supplied it must verify against the configured secret.
func TestVerifySignature_Generic_OptInHMAC(t *testing.T) {
	body := []byte(`{"event":"push"}`)
	secret := "generic-secret"
	good := signPayload(body, secret) // raw hex

	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{"no signature header → accepted (non-breaking)", map[string]string{}, true},
		{"valid X-Signature-256 raw hex", map[string]string{"X-Signature-256": good}, true},
		{"valid X-Signature-256 sha256= prefix", map[string]string{"X-Signature-256": "sha256=" + good}, true},
		{"invalid X-Signature-256", map[string]string{"X-Signature-256": "deadbeef"}, false},
		{"valid X-Hub-Signature-256", map[string]string{"X-Hub-Signature-256": "sha256=" + good}, true},
		{"wrong secret signature", map[string]string{"X-Signature-256": signPayload(body, "other")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := VerifySignature(context.Background(), "generic", body, secret, r); got != tc.want {
				t.Errorf("VerifySignature = %v, want %v", got, tc.want)
			}
		})
	}
}

// statefulBolt is an in-memory core.BoltStorer for the replay-dedup test.
type statefulBolt struct {
	secret string
	data   map[string][]byte
}

func (m *statefulBolt) Set(ctx context.Context, bucket, key string, value any, _ int64) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[bucket+"/"+key] = b
	return nil
}

func (m *statefulBolt) Get(ctx context.Context, bucket, key string, dest any) error {
	b, ok := m.data[bucket+"/"+key]
	if !ok {
		return core.ErrKVNotFound
	}
	return json.Unmarshal(b, dest)
}

func (m *statefulBolt) BatchSet(ctx context.Context, _ []core.BoltBatchItem) error { return nil }
func (m *statefulBolt) Delete(ctx context.Context, _, _ string) error              { return nil }
func (m *statefulBolt) List(ctx context.Context, _ string) ([]string, error)       { return nil, nil }
func (m *statefulBolt) Close() error                          { return nil }
func (m *statefulBolt) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, core.ErrKVNotFound
}
func (m *statefulBolt) GetWebhookSecret(_ string) (string, error) { return m.secret, nil }

// P1-14: replay defense. A delivery already processed within the dedup window
// is acked with 200 {"status":"duplicate"} and must not re-emit the event.
func TestHandleWebhook_RejectsReplayedDelivery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)

	var received int
	events.Subscribe(core.EventWebhookReceived, func(_ context.Context, _ core.Event) error {
		received++
		return nil
	})

	bolt := &statefulBolt{secret: "s"}
	recv := NewReceiver(nil, bolt, events, logger)

	send := func() *httptest.ResponseRecorder {
		body := `{"ref":"refs/heads/main"}`
		req := httptest.NewRequest("POST", "/hooks/v1/wh-1", strings.NewReader(body))
		req.SetPathValue("webhookID", "wh-1")
		req.Header.Set("X-Request-Id", "delivery-abc") // generic provider + delivery id
		rr := httptest.NewRecorder()
		recv.HandleWebhook(rr, req)
		return rr
	}

	first := send()
	if first.Code != http.StatusOK {
		t.Fatalf("first delivery: expected 200, got %d", first.Code)
	}

	second := send()
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate delivery: expected 200, got %d", second.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(second.Body).Decode(&resp)
	if resp["status"] != "duplicate" {
		t.Errorf("duplicate delivery status = %q, want duplicate", resp["status"])
	}

	if received != 1 {
		t.Errorf("webhook.received emitted %d times, want 1 (duplicate must not re-trigger)", received)
	}
}
