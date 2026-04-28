package webhooks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// =============================================================================
// DeliveryTracker — 0% coverage file
// =============================================================================

type mockBoltStoreDelivery struct {
	mu        sync.Mutex
	setCalled bool
	lastKey   string
	lastVal   any
}

func (m *mockBoltStoreDelivery) Set(_, key string, val any, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalled = true
	m.lastKey = key
	m.lastVal = val
	return nil
}

func (m *mockBoltStoreDelivery) wasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setCalled
}

func (m *mockBoltStoreDelivery) lastValue() any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastVal
}
func (m *mockBoltStoreDelivery) BatchSet(_ []core.BoltBatchItem) error                   { return nil }
func (m *mockBoltStoreDelivery) Get(_, _ string, _ any) error                            { return nil }
func (m *mockBoltStoreDelivery) Delete(_, _ string) error                                { return nil }
func (m *mockBoltStoreDelivery) List(_ string) ([]string, error)                         { return nil, nil }
func (m *mockBoltStoreDelivery) Close() error                                            { return nil }
func (m *mockBoltStoreDelivery) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, nil
}
func (m *mockBoltStoreDelivery) GetWebhookSecret(_ string) (string, error) { return "", nil }

func TestNewDeliveryTracker(t *testing.T) {
	bolt := &mockBoltStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(bolt, events)
	if dt == nil {
		t.Fatal("NewDeliveryTracker returned nil")
	}
}

func TestDeliveryTracker_Start_SentEvent(t *testing.T) {
	bolt := &mockBoltStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(bolt, events)
	dt.Start()

	events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", core.NotificationEventData{
		Recipient: "https://example.com/hook",
	})

	// Async handler — give it a moment
	time.Sleep(50 * time.Millisecond)

	if !bolt.wasCalled() {
		t.Error("record should have been called for sent event")
	}
	log, ok := bolt.lastValue().(DeliveryLog)
	if !ok {
		t.Fatalf("expected DeliveryLog, got %T", bolt.lastValue())
	}
	if log.Status != "sent" {
		t.Errorf("status = %q, want sent", log.Status)
	}
	if log.URL != "https://example.com/hook" {
		t.Errorf("url = %q, want https://example.com/hook", log.URL)
	}
}

func TestDeliveryTracker_Start_FailedEvent(t *testing.T) {
	bolt := &mockBoltStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(bolt, events)
	dt.Start()

	events.EmitWithTenant(context.Background(), core.EventOutboundFailed, "webhook", "t1", "u1", core.NotificationEventData{
		Recipient: "https://example.com/hook",
		Error:     "connection refused",
	})

	time.Sleep(50 * time.Millisecond)

	if !bolt.wasCalled() {
		t.Error("record should have been called for failed event")
	}
	log, ok := bolt.lastValue().(DeliveryLog)
	if !ok {
		t.Fatalf("expected DeliveryLog, got %T", bolt.lastValue())
	}
	if log.Status != "failed" {
		t.Errorf("status = %q, want failed", log.Status)
	}
	if log.Error != "connection refused" {
		t.Errorf("error = %q, want connection refused", log.Error)
	}
}

func TestDeliveryTracker_Start_WrongDataType(t *testing.T) {
	bolt := &mockBoltStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(bolt, events)
	dt.Start()

	// Emit with wrong data type — handler should return nil without recording
	events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", "not-notification-data")

	time.Sleep(50 * time.Millisecond)

	if bolt.wasCalled() {
		t.Error("record should NOT have been called for wrong data type")
	}
}

// =============================================================================
// parseBitbucket — flat envelope fallback path
// =============================================================================

func TestParseBitbucket_NativePush(t *testing.T) {
	body := []byte(`{
		"push": {"changes": [{"new": {"name": "main", "target": {"hash": "abc123", "message": "fix it", "author": {"raw": "Dev <dev@x.com>"}}}}]},
		"repository": {"full_name": "org/repo", "links": {"clone": [{"name": "https", "href": "https://bb.com/org/repo.git"}]}}
	}`)
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Event-Key", "repo:push")

	p, err := parseBitbucket(body, r)
	if err != nil {
		t.Fatalf("parseBitbucket: %v", err)
	}
	if p.Provider != "bitbucket" {
		t.Errorf("provider = %q, want bitbucket", p.Provider)
	}
	if p.Branch != "main" {
		t.Errorf("branch = %q, want main", p.Branch)
	}
	if p.CommitSHA != "abc123" {
		t.Errorf("commit = %q, want abc123", p.CommitSHA)
	}
	if p.RepoName != "org/repo" {
		t.Errorf("repo_name = %q, want org/repo", p.RepoName)
	}
}

func TestParseBitbucket_FlatEnvelopeFallback(t *testing.T) {
	body := []byte(`{"provider":"bitbucket","event_type":"repo:push","branch":"develop","commit_sha":"def456","commit_message":"feat x","author":"Alice","repo_url":"https://bb.com/a/b.git","repo_name":"a/b"}`)
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Event-Key", "repo:push")

	p, err := parseBitbucket(body, r)
	if err != nil {
		t.Fatalf("parseBitbucket: %v", err)
	}
	if p.Branch != "develop" {
		t.Errorf("branch = %q, want develop", p.Branch)
	}
	if p.CommitSHA != "def456" {
		t.Errorf("commit = %q, want def456", p.CommitSHA)
	}
	if p.Author != "Alice" {
		t.Errorf("author = %q, want Alice", p.Author)
	}
}

func TestParseBitbucket_InvalidJSON(t *testing.T) {
	_, err := parseBitbucket([]byte(`not json`), &http.Request{Header: http.Header{}})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// =============================================================================
// VerifyBitbucketSignature
// =============================================================================

func TestVerifyBitbucketSignature_Valid(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "bb-secret"
	sig := "sha256=" + signPayload(body, secret)

	if !VerifyBitbucketSignature(body, secret, sig) {
		t.Error("valid signature should pass")
	}
}

func TestVerifyBitbucketSignature_RawHex(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "bb-secret"
	sig := signPayload(body, secret) // without sha256= prefix

	if !VerifyBitbucketSignature(body, secret, sig) {
		t.Error("raw hex signature should pass")
	}
}

func TestVerifyBitbucketSignature_Empty(t *testing.T) {
	if VerifyBitbucketSignature([]byte("x"), "s", "") {
		t.Error("empty signature should fail")
	}
}

func TestVerifyBitbucketSignature_WrongSecret(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "bb-secret"
	sig := "sha256=" + signPayload(body, secret)

	if VerifyBitbucketSignature(body, "wrong", sig) {
		t.Error("wrong secret should fail")
	}
}

// =============================================================================
// VerifySignature — bitbucket with X-Hub-Signature header
// =============================================================================

func TestVerifySignature_BitbucketWithSignature(t *testing.T) {
	body := []byte(`{"push":{"changes":[]}}`)
	secret := "bb-test-secret"
	sig := "sha256=" + signPayload(body, secret)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Hub-Signature", sig)

	if !VerifySignature(context.Background(), "bitbucket", body, secret, r) {
		t.Error("bitbucket with valid X-Hub-Signature should pass")
	}
}

func TestVerifySignature_BitbucketWithBadSignature(t *testing.T) {
	body := []byte(`{"push":{"changes":[]}}`)
	secret := "bb-test-secret"
	sig := "sha256=" + signPayload(body, "wrong-secret")

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Hub-Signature", sig)

	if VerifySignature(context.Background(), "bitbucket", body, secret, r) {
		t.Error("bitbucket with invalid X-Hub-Signature should fail")
	}
}

func TestVerifySignature_BitbucketCloudNoSignature(t *testing.T) {
	body := []byte(`{"push":{"changes":[]}}`)
	r := &http.Request{Header: http.Header{}}
	// No X-Hub-Signature header — Bitbucket Cloud path

	if !VerifySignature(context.Background(), "bitbucket", body, "any", r) {
		t.Error("bitbucket without signature header should pass (Cloud)")
	}
}

// =============================================================================
// HandleWebhook — bitbucket full path with repository links
// =============================================================================

func TestHandleWebhook_BitbucketCloud_Push(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	recv := NewReceiver(nil, nil, events, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	body := `{"push":{"changes":[{"new":{"name":"develop","target":{"hash":"bb999","message":"wip","author":{"raw":"Bob"}}}}]},"repository":{"full_name":"team/repo","links":{"clone":[{"name":"https","href":"https://bitbucket.org/team/repo.git"}]}}}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-bb", strings.NewReader(body))
	req.Header.Set("X-Event-Key", "repo:push")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
