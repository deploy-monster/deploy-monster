package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// === merged from coverage_boost_test.go ===

// =============================================================================
// DeliveryTracker — 0% coverage file
// =============================================================================

type mockKVStoreDelivery struct {
	mu        sync.Mutex
	setCalled bool
	lastKey   string
	lastVal   any
}

func (m *mockKVStoreDelivery) Set(_, key string, val any, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalled = true
	m.lastKey = key
	m.lastVal = val
	return nil
}

func (m *mockKVStoreDelivery) wasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setCalled
}

func (m *mockKVStoreDelivery) lastValue() any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastVal
}
func (m *mockKVStoreDelivery) BatchSet(_ []core.KVBatchItem) error { return nil }
func (m *mockKVStoreDelivery) Get(_, _ string, _ any) error        { return nil }
func (m *mockKVStoreDelivery) Delete(_, _ string) error            { return nil }
func (m *mockKVStoreDelivery) List(_ string) ([]string, error)     { return nil, nil }
func (m *mockKVStoreDelivery) Close() error                        { return nil }
func (m *mockKVStoreDelivery) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, nil
}
func (m *mockKVStoreDelivery) GetWebhookSecret(_ string) (string, error) { return "", nil }

func TestNewDeliveryTracker(t *testing.T) {
	kv := &mockKVStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(kv, events)
	if dt == nil {
		t.Fatal("NewDeliveryTracker returned nil")
	}
}

func TestDeliveryTracker_Start_MissingDependencies(t *testing.T) {
	NewDeliveryTracker(nil, nil).Start()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	NewDeliveryTracker(nil, events).Start()

	events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", core.NotificationEventData{
		Recipient: "https://example.com/hook",
	})
	events.Drain()
}

func TestDeliveryTracker_Start_SentEvent(t *testing.T) {
	kv := &mockKVStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(kv, events)
	dt.Start()

	events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", core.NotificationEventData{
		Recipient: "https://example.com/hook",
	})

	// Async handler — give it a moment
	time.Sleep(50 * time.Millisecond)

	if !kv.wasCalled() {
		t.Error("record should have been called for sent event")
	}
	log, ok := kv.lastValue().(DeliveryLog)
	if !ok {
		t.Fatalf("expected DeliveryLog, got %T", kv.lastValue())
	}
	if log.Status != "sent" {
		t.Errorf("status = %q, want sent", log.Status)
	}
	if log.URL != "https://example.com/hook" {
		t.Errorf("url = %q, want https://example.com/hook", log.URL)
	}
	if log.TenantID != "t1" || log.UserID != "u1" {
		t.Errorf("tenant/user = %q/%q, want t1/u1", log.TenantID, log.UserID)
	}
}

func TestDeliveryTracker_Start_FailedEvent(t *testing.T) {
	kv := &mockKVStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(kv, events)
	dt.Start()

	events.EmitWithTenant(context.Background(), core.EventOutboundFailed, "webhook", "t1", "u1", core.NotificationEventData{
		Recipient: "https://example.com/hook",
		Error:     "connection refused",
	})

	time.Sleep(50 * time.Millisecond)

	if !kv.wasCalled() {
		t.Error("record should have been called for failed event")
	}
	log, ok := kv.lastValue().(DeliveryLog)
	if !ok {
		t.Fatalf("expected DeliveryLog, got %T", kv.lastValue())
	}
	if log.Status != "failed" {
		t.Errorf("status = %q, want failed", log.Status)
	}
	if log.Error != "connection refused" {
		t.Errorf("error = %q, want connection refused", log.Error)
	}
	if log.TenantID != "t1" || log.UserID != "u1" {
		t.Errorf("tenant/user = %q/%q, want t1/u1", log.TenantID, log.UserID)
	}
}

func TestDeliveryTracker_Start_WrongDataType(t *testing.T) {
	kv := &mockKVStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(kv, events)
	dt.Start()

	// Emit with wrong data type — handler should return nil without recording
	events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", "not-notification-data")

	time.Sleep(50 * time.Millisecond)

	if kv.wasCalled() {
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

// === merged from coverage_final_test.go ===

// =============================================================================
// DeliveryTracker.record — nil kv path (delivery_log.go:73)
// =============================================================================

// TestDeliveryTracker_Record_NilBolt covers the nil-kv early return in
// record(). Create a tracker with kv=nil, then call Start(), which returns
// immediately because kv is nil. Then call record directly (it's
// accessible because we're in the same package).
func TestDeliveryTracker_Record_NilBolt(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(nil, events)
	// record with nil kv should return nil (no-op)
	err := dt.record(DeliveryLog{
		ID:     "test-id",
		URL:    "https://example.com/hook",
		Status: "sent",
	})
	if err != nil {
		t.Errorf("record with nil kv should return nil, got: %v", err)
	}
}

// =============================================================================
// DeliveryTracker.Start — EventOutboundFailed with wrong data type
// (delivery_log.go:57-59)
// =============================================================================

// TestDeliveryTracker_Start_FailedWrongDataType covers the !ok path in the
// EventOutboundFailed subscription handler.
func TestDeliveryTracker_Start_FailedWrongDataType(t *testing.T) {
	kv := &mockKVStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(kv, events)
	dt.Start()

	// Emit EventOutboundFailed with wrong data type — handler should return
	// nil without recording (the !ok path).
	events.EmitWithTenant(context.Background(), core.EventOutboundFailed, "webhook", "t1", "u1", "not-notification-data")

	// Drain async handlers
	events.Drain()

	if kv.wasCalled() {
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
func (m *boltSetFailing) BatchSet(_ []core.KVBatchItem) error   { return nil }
func (m *boltSetFailing) Get(_, _ string, _ any) error {
	// Return ErrKVNotFound so dedup proceeds to Set
	return core.ErrKVNotFound
}
func (m *boltSetFailing) Delete(_, _ string) error        { return nil }
func (m *boltSetFailing) List(_ string) ([]string, error) { return nil, nil }
func (m *boltSetFailing) Close() error                    { return nil }
func (m *boltSetFailing) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, core.ErrKVNotFound
}
func (m *boltSetFailing) GetWebhookSecret(_ string) (string, error) { return m.secret, nil }

func TestHandleWebhook_DedupSetError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	kv := &boltSetFailing{secret: "s", setErr: fmt.Errorf("dedup write failed")}
	recv := NewReceiver(nil, kv, events, logger)

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

	kv := &statefulBolt{secret: "s"}
	recv := NewReceiver(nil, kv, events, logger)

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
// DeliveryTracker — concurrent use of mockKVStoreDelivery (bonus: race check)
// =============================================================================

func TestDeliveryTracker_Concurrent(t *testing.T) {
	kv := &mockKVStoreDelivery{}
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dt := NewDeliveryTracker(kv, events)
	dt.Start()

	// Emit events concurrently — wait for all goroutines before draining
	// so the race detector doesn't flag the test goroutine finishing while
	// emit goroutines are still accessing the EventBus.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			events.EmitWithTenant(context.Background(), core.EventOutboundSent, "webhook", "t1", "u1", core.NotificationEventData{
				Recipient: fmt.Sprintf("https://example%d.com/hook", n),
			})
		}(i)
	}
	wg.Wait()

	events.Drain()
}

// === merged from final90_test.go ===

// =============================================================================
// HandleWebhook — io.ReadAll error path (body returns read error)
// =============================================================================

type errReader struct{}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestHandleWebhook_ReadBodyError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	recv := NewReceiver(nil, nil, events, logger)

	req := httptest.NewRequest("POST", "/hooks/v1/wh-1", &errReader{})
	req.SetPathValue("webhookID", "wh-1")
	rr := httptest.NewRecorder()

	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to read body") {
		t.Errorf("expected 'failed to read body' error, got: %s", rr.Body.String())
	}
}

// =============================================================================
// HandleWebhook — Bitbucket provider path through mux (exercises full path)
// =============================================================================

func TestHandleWebhook_BitbucketPush_FullPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	recv := NewReceiver(nil, nil, events, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	body := `{"push":{"changes":[{"new":{"name":"main","type":"branch","target":{"hash":"bb1234"}}}]}}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-bb-full", strings.NewReader(body))
	req.Header.Set("X-Event-Key", "repo:push")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// VerifySignature — GitHub provider path
// =============================================================================

func TestVerifySignature_GitHubValid(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "gh-test-secret"
	sig := "sha256=" + signPayload(body, secret)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Hub-Signature-256", sig)

	if !VerifySignature(context.Background(), "github", body, secret, req) {
		t.Error("expected valid signature to pass")
	}
}

func TestVerifySignature_GitLabValid(t *testing.T) {
	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Gitlab-Token", "my-token")

	if !VerifySignature(context.Background(), "gitlab", nil, "my-token", req) {
		t.Error("expected matching token to pass")
	}
}

// =============================================================================
// Receiver — NewReceiver fields
// =============================================================================

func TestNewReceiver_FieldsSet(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)

	r := NewReceiver(nil, nil, events, logger)
	if r == nil {
		t.Fatal("NewReceiver returned nil")
	}
	if r.events != events {
		t.Error("events not set")
	}
	if r.logger != logger {
		t.Error("logger not set")
	}
}

// =============================================================================
// HandleWebhook — parse error (malformed JSON for generic provider)
// =============================================================================

func TestHandleWebhook_ParseError_Generic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	recv := NewReceiver(nil, nil, events, logger)

	// No provider headers => generic, but body is not valid JSON
	req := httptest.NewRequest("POST", "/hooks/v1/wh-parse", strings.NewReader("not json at all{{{"))
	req.SetPathValue("webhookID", "wh-parse")
	rr := httptest.NewRecorder()

	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid payload") {
		t.Errorf("expected 'invalid payload' in response, got: %s", rr.Body.String())
	}
}

// =============================================================================
// HandleWebhook - Signature Verification Tests
// =============================================================================

func TestHandleWebhook_SignatureVerification_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	kv := &mockBoltWithSecret{secret: "test-secret"}
	recv := NewReceiver(nil, kv, events, logger)

	body := `{"ref":"refs/heads/main"}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-test", strings.NewReader(body))
	req.SetPathValue("webhookID", "wh-test")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", computeTestSignature(body, "test-secret"))
	rr := httptest.NewRecorder()
	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with valid signature, got %d", rr.Code)
	}
}

func TestHandleWebhook_SignatureVerification_Failed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	kv := &mockBoltWithSecret{secret: "correct-secret"}
	recv := NewReceiver(nil, kv, events, logger)

	body := `{"ref":"refs/heads/main"}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-test", strings.NewReader(body))
	req.SetPathValue("webhookID", "wh-test")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", computeTestSignature(body, "wrong-secret"))
	rr := httptest.NewRecorder()
	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid signature, got %d", rr.Code)
	}
}

func TestHandleWebhook_SecretLookupFailed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	kv := &mockBoltWithSecret{err: fmt.Errorf("secret not found")}
	recv := NewReceiver(nil, kv, events, logger)

	body := `{"ref":"refs/heads/main"}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-test", strings.NewReader(body))
	req.SetPathValue("webhookID", "wh-test")
	req.Header.Set("X-GitHub-Event", "push")
	rr := httptest.NewRecorder()
	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when secret lookup fails, got %d", rr.Code)
	}
}

// mockBoltWithSecret is a mock that returns a webhook secret
type mockBoltWithSecret struct {
	secret string
	err    error
}

func (m *mockBoltWithSecret) Set(_, _ string, _ any, _ int64) error { return nil }
func (m *mockBoltWithSecret) BatchSet(_ []core.KVBatchItem) error   { return nil }
func (m *mockBoltWithSecret) Get(_, _ string, _ any) error          { return fmt.Errorf("not found") }
func (m *mockBoltWithSecret) Delete(_, _ string) error              { return nil }
func (m *mockBoltWithSecret) List(_ string) ([]string, error)       { return nil, nil }
func (m *mockBoltWithSecret) Close() error                          { return nil }
func (m *mockBoltWithSecret) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockBoltWithSecret) GetWebhookSecret(_ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.secret, nil
}

func computeTestSignature(body, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
