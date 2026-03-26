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
)

// =============================================================================
// Coverage targets:
//   outbound.go:39  Send    97.1% — small gap, likely the timeout branch (lines 52-56)
//   pipeline.go:38  Trigger 40.9% — large gap: build success path, deploy, event publish
//
// The Pipeline.Trigger function at 40.9% has these uncovered paths:
//   - Lines 50-51: store.UpdateAppStatus("building")
//   - Lines 54-60: builder.Build success + result handling
//   - Lines 62-63: store.UpdateAppStatus("failed") on build error (already covered by TestPipeline_Trigger_BuildFails)
//   - Lines 67-98: deploy image, create container, publish event
//   - Lines 100-118: success path with event publish
//
// The build ALWAYS fails in tests because there's no real git repo / docker.
// To cover the deploy path, we need a mock builder that returns success.
// Since build.Builder is a concrete type (not an interface), we cannot mock it
// without modifying the source. The best we can do is test the Trigger paths
// that run before the build, and the outbound Send timeout path.
// =============================================================================

// ---------------------------------------------------------------------------
// OutboundSender.Send — custom timeout path (lines 52-56)
// ---------------------------------------------------------------------------

func TestFinal_OutboundSender_Send_WithTimeout(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Method:  http.MethodPost,
		Payload: map[string]string{"event": "deploy.completed"},
		Secret:  "test-secret",
		Timeout: 5, // 5 seconds — exercises the timeout path (lines 52-56)
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !received {
		t.Error("server should have received the request")
	}
}

func TestFinal_OutboundSender_Send_ZeroTimeout(t *testing.T) {
	// Timeout=0 means no custom timeout is applied — uses default client timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Payload: map[string]string{"event": "test"},
		Timeout: 0, // 0 means no custom timeout — exercises the `if timeout > 0` false branch
	})
	if err != nil {
		t.Fatalf("Send with timeout=0: %v", err)
	}
}

func TestFinal_OutboundSender_Send_WithPositiveTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	// Positive timeout value exercises the context.WithTimeout path
	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Payload: map[string]string{"key": "value"},
		Timeout: 10, // 10 seconds
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
}

// ---------------------------------------------------------------------------
// OutboundSender.Send — GET method
// ---------------------------------------------------------------------------

func TestFinal_OutboundSender_Send_GetMethod(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Method:  http.MethodPut,
		Payload: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
}

// ---------------------------------------------------------------------------
// OutboundSender.Send — default method (empty = POST)
// ---------------------------------------------------------------------------

func TestFinal_OutboundSender_Send_DefaultMethod(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Payload: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST (default)", gotMethod)
	}
}

// ---------------------------------------------------------------------------
// OutboundSender.Send — no events bus (nil check)
// ---------------------------------------------------------------------------

func TestFinal_OutboundSender_Send_NilEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewOutboundSender(nil)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Payload: map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Send with nil events: %v", err)
	}
}

func TestFinal_OutboundSender_Send_NilEvents_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender := NewOutboundSender(nil)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Payload: map[string]string{"key": "value"},
	})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// ---------------------------------------------------------------------------
// Pipeline.Trigger — exercises GetApp, UpdateAppStatus, then build fails
// This covers lines 39-63 (everything before the successful build path).
// ---------------------------------------------------------------------------

func TestFinal_Pipeline_Trigger_AppFoundBuildFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &finalMockStore{
		app: &core.Application{
			ID:        "app-1",
			Name:      "test-app",
			TenantID:  "t1",
			SourceURL: "https://github.com/org/repo.git",
			Branch:    "main",
		},
	}
	runtime := &finalMockRuntime{}

	p := NewPipeline(store, runtime, events, logger)

	err := p.Trigger(context.Background(), "app-1", &WebhookPayload{
		Branch:    "main",
		CommitSHA: "abc123def",
	})
	// Build will fail because there's no real git repo to clone
	if err == nil {
		t.Fatal("expected error from build pipeline")
	}
	if !strings.Contains(err.Error(), "build failed") {
		t.Errorf("expected 'build failed' error, got: %v", err)
	}

	// Verify UpdateAppStatus was called with "building" before build
	if !store.statusUpdated {
		t.Error("expected UpdateAppStatus to be called")
	}
}

func TestFinal_Pipeline_Trigger_GetAppError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &finalMockStore{getAppErr: fmt.Errorf("database connection failed")}
	runtime := &finalMockRuntime{}

	p := NewPipeline(store, runtime, events, logger)

	err := p.Trigger(context.Background(), "bad-app", &WebhookPayload{
		Branch: "main",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "get app") {
		t.Errorf("expected 'get app' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// parsePayload — Bitbucket provider with push changes
// ---------------------------------------------------------------------------

func TestFinal_ParsePayload_Bitbucket(t *testing.T) {
	// Bitbucket falls through to parseGeneric since there's no parseBitbucket.
	// The provider field comes from the JSON payload, not the header detection.
	body := []byte(`{
		"provider": "bitbucket",
		"branch": "main",
		"commit_sha": "bitbucket123"
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Event-Key", "repo:push")

	payload, err := parsePayload("bitbucket", body, req)
	if err != nil {
		t.Fatalf("parsePayload bitbucket: %v", err)
	}
	if payload.Provider != "bitbucket" {
		t.Errorf("provider = %q, want bitbucket", payload.Provider)
	}
	if payload.Branch != "main" {
		t.Errorf("branch = %q, want main", payload.Branch)
	}
}

// ---------------------------------------------------------------------------
// OutboundSender.Send — verify HMAC signature header
// ---------------------------------------------------------------------------

func TestFinal_OutboundSender_Send_VerifySignatureHeader(t *testing.T) {
	var gotSig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Monster-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	payload := map[string]string{"event": "app.deployed"}
	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     server.URL,
		Payload: payload,
		Secret:  "hmac-test-secret",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !strings.HasPrefix(gotSig, "sha256=") {
		t.Errorf("X-Monster-Signature = %q, want sha256= prefix", gotSig)
	}

	// Verify the signature is correct
	body, _ := json.Marshal(payload)
	expected := signPayload(body, "hmac-test-secret")
	if gotSig != "sha256="+expected {
		t.Errorf("signature mismatch")
	}
}

// ---------------------------------------------------------------------------
// OutboundSender.Send — marshal error (nil payload)
// ---------------------------------------------------------------------------

func TestFinal_OutboundSender_Send_InvalidPayload(t *testing.T) {
	events := core.NewEventBus(nil)
	sender := NewOutboundSender(events)

	// A channel cannot be marshaled to JSON
	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     "http://example.com",
		Payload: make(chan int), // unmarshalable
	})
	if err == nil {
		t.Fatal("expected error for unmarshalable payload")
	}
	if !strings.Contains(err.Error(), "marshal payload") {
		t.Errorf("expected 'marshal payload' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// parsePayload — GitHub with full commit data
// ---------------------------------------------------------------------------

func TestFinal_ParsePayload_GitHub_FullCommit(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/feature-branch",
		"head_commit": {
			"id": "gh-commit-sha",
			"message": "feat: add new feature",
			"author": {"name": "Dev User"}
		},
		"repository": {
			"clone_url": "https://github.com/org/repo.git",
			"full_name": "org/repo"
		}
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-GitHub-Event", "push")

	payload, err := parsePayload("github", body, req)
	if err != nil {
		t.Fatalf("parsePayload: %v", err)
	}

	if payload.Provider != "github" {
		t.Errorf("provider = %q", payload.Provider)
	}
	if payload.Branch != "feature-branch" {
		t.Errorf("branch = %q", payload.Branch)
	}
	if payload.CommitSHA != "gh-commit-sha" {
		t.Errorf("commit_sha = %q", payload.CommitSHA)
	}
	if payload.CommitMsg != "feat: add new feature" {
		t.Errorf("commit_msg = %q", payload.CommitMsg)
	}
	if payload.Author != "Dev User" {
		t.Errorf("author = %q", payload.Author)
	}
	if payload.RepoURL != "https://github.com/org/repo.git" {
		t.Errorf("repo_url = %q", payload.RepoURL)
	}
}

// ---------------------------------------------------------------------------
// VerifySignature — Gitea/Gogs paths
// ---------------------------------------------------------------------------

func TestFinal_VerifySignature_Gitea(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "gitea-secret"
	sig := computeHMACSHA256(body, secret)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Gitea-Signature", sig)

	if !VerifySignature(context.Background(), "gitea", body, secret, req) {
		t.Error("expected valid gitea signature to pass")
	}
}

func TestFinal_VerifySignature_Gogs(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "gogs-secret"
	sig := computeHMACSHA256(body, secret)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Gogs-Signature", sig)

	if !VerifySignature(context.Background(), "gogs", body, secret, req) {
		t.Error("expected valid gogs signature to pass")
	}
}

func TestFinal_VerifySignature_GitHubInvalid(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	if VerifySignature(context.Background(), "github", body, "correct-secret", req) {
		t.Error("expected invalid signature to fail")
	}
}

// ---------------------------------------------------------------------------
// Receiver.HandleWebhook — missing webhookID
// ---------------------------------------------------------------------------

func TestFinal_Receiver_HandleWebhook_MissingWebhookID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	recv := NewReceiver(nil, events, logger)

	req := httptest.NewRequest("POST", "/hooks/v1/", strings.NewReader(`{}`))
	// No webhookID path value set
	rr := httptest.NewRecorder()

	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "missing webhook ID") {
		t.Errorf("expected 'missing webhook ID', got: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// parseBitbucket — direct call
// ---------------------------------------------------------------------------

func TestFinal_ParsePayload_Bitbucket_MalformedJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/hooks/v1/wh", nil)
	req.Header.Set("X-Event-Key", "repo:push")

	_, err := parsePayload("bitbucket", []byte(`not json`), req)
	if err != nil {
		// Bitbucket parse might return an error for malformed JSON
		t.Logf("expected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock store for pipeline tests (unique to this file)
// ---------------------------------------------------------------------------

type finalMockStore struct {
	app           *core.Application
	getAppErr     error
	statusUpdated bool
}

func (m *finalMockStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	if m.getAppErr != nil {
		return nil, m.getAppErr
	}
	if m.app != nil {
		return m.app, nil
	}
	return &core.Application{ID: "app-1", Name: "test-app", TenantID: "t1"}, nil
}

func (m *finalMockStore) UpdateAppStatus(_ context.Context, _, status string) error {
	m.statusUpdated = true
	return nil
}

func (m *finalMockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}

func (m *finalMockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error {
	return nil
}

func (m *finalMockStore) Close() error                 { return nil }
func (m *finalMockStore) Ping(_ context.Context) error { return nil }
func (m *finalMockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) CreateUser(_ context.Context, _ *core.User) error    { return nil }
func (m *finalMockStore) UpdateUser(_ context.Context, _ *core.User) error    { return nil }
func (m *finalMockStore) UpdatePassword(_ context.Context, _, _ string) error { return nil }
func (m *finalMockStore) UpdateLastLogin(_ context.Context, _ string) error   { return nil }
func (m *finalMockStore) CountUsers(_ context.Context) (int, error)           { return 0, nil }
func (m *finalMockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *finalMockStore) CreateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (m *finalMockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (m *finalMockStore) DeleteTenant(_ context.Context, _ string) error       { return nil }
func (m *finalMockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *finalMockStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *finalMockStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *finalMockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (m *finalMockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (m *finalMockStore) DeleteApp(_ context.Context, _ string) error          { return nil }
func (m *finalMockStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (m *finalMockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (m *finalMockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	return nil, nil
}
func (m *finalMockStore) DeleteDomain(_ context.Context, _ string) error { return nil }
func (m *finalMockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (m *finalMockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}
func (m *finalMockStore) CreateProject(_ context.Context, _ *core.Project) error { return nil }
func (m *finalMockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (m *finalMockStore) DeleteProject(_ context.Context, _ string) error            { return nil }
func (m *finalMockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (m *finalMockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *finalMockStore) CreateSecret(_ context.Context, _ *core.Secret) error { return nil }
func (m *finalMockStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}
func (m *finalMockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (m *finalMockStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, core.ErrNotFound
}
func (m *finalMockStore) CreateInvite(_ context.Context, _ *core.Invitation) error { return nil }
func (m *finalMockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (m *finalMockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}

// finalMockRuntime implements core.ContainerRuntime.
type finalMockRuntime struct{}

func (m *finalMockRuntime) Ping() error { return nil }
func (m *finalMockRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "container-id", nil
}
func (m *finalMockRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *finalMockRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *finalMockRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *finalMockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *finalMockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (m *finalMockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *finalMockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *finalMockRuntime) ImagePull(_ context.Context, _ string) error           { return nil }
func (m *finalMockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (m *finalMockRuntime) ImageRemove(_ context.Context, _ string) error         { return nil }
func (m *finalMockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *finalMockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}
