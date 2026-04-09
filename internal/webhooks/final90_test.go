package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
// Pipeline.Trigger — app found but build fails (no real docker/git)
// =============================================================================

func TestPipeline_Trigger_BuildFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &pipelineMockStore{} // returns a valid app
	runtime := &pipelineMockRuntime{}

	p := NewPipeline(store, runtime, events, logger)

	err := p.Trigger(context.Background(), "app-1", &WebhookPayload{
		Branch:    "main",
		CommitSHA: "abc123",
	})
	// Build will fail because SourceURL is empty / no real git repo
	if err == nil {
		t.Fatal("expected error from build pipeline (no real git/docker)")
	}
	if !strings.Contains(err.Error(), "build failed") {
		t.Errorf("expected 'build failed' error, got: %v", err)
	}
}

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
	recv.RegisterRoutes(mux)

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
	sig := "sha256=" + computeHMACSHA256(body, secret)

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
	store := &pipelineMockStore{}

	r := NewReceiver(store, nil, events, logger)
	if r == nil {
		t.Fatal("NewReceiver returned nil")
	}
	if r.store != store {
		t.Error("store not set")
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
	bolt := &mockBoltWithSecret{secret: "test-secret"}
	recv := NewReceiver(nil, bolt, events, logger)

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
	bolt := &mockBoltWithSecret{secret: "correct-secret"}
	recv := NewReceiver(nil, bolt, events, logger)

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
	bolt := &mockBoltWithSecret{err: fmt.Errorf("secret not found")}
	recv := NewReceiver(nil, bolt, events, logger)

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

func (m *mockBoltWithSecret) Set(_, _ string, _ any, _ int64) error    { return nil }
func (m *mockBoltWithSecret) BatchSet(_ []core.BoltBatchItem) error    { return nil }
func (m *mockBoltWithSecret) Get(_, _ string, _ any) error { return fmt.Errorf("not found") }
func (m *mockBoltWithSecret) Delete(_, _ string) error { return nil }
func (m *mockBoltWithSecret) List(_ string) ([]string, error) { return nil, nil }
func (m *mockBoltWithSecret) Close() error { return nil }
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
