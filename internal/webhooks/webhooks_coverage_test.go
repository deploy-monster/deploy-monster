package webhooks

import (
	"context"
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
// Pipeline — NewPipeline, Trigger (app-not-found path)
// =============================================================================

func TestNewPipeline_Created(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &pipelineMockStore{}
	runtime := &pipelineMockRuntime{}

	p := NewPipeline(store, runtime, events, logger)
	if p == nil {
		t.Fatal("NewPipeline returned nil")
	}
	if p.store != store {
		t.Error("store not set")
	}
	if p.runtime != runtime {
		t.Error("runtime not set")
	}
	if p.events != events {
		t.Error("events not set")
	}
	if p.builder == nil {
		t.Error("builder should be initialized")
	}
}

func TestPipeline_Trigger_AppNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &pipelineMockStore{getAppErr: core.ErrNotFound}
	runtime := &pipelineMockRuntime{}

	p := NewPipeline(store, runtime, events, logger)

	err := p.Trigger(context.Background(), "nonexistent-app", &WebhookPayload{
		Branch:    "main",
		CommitSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for missing app")
	}
}

// TestPipeline_Trigger_GetAppError covers the store error path
func TestPipeline_Trigger_GetAppError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &pipelineMockStore{getAppErr: fmt.Errorf("database connection lost")}
	runtime := &pipelineMockRuntime{}

	p := NewPipeline(store, runtime, events, logger)

	err := p.Trigger(context.Background(), "app-1", &WebhookPayload{
		Branch:    "main",
		CommitSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error from GetApp")
	}
	if !strings.Contains(err.Error(), "get app") {
		t.Errorf("error should contain 'get app', got: %v", err)
	}
}

// =============================================================================
// Receiver — Bitbucket detection, parseGitLab, parseGitea, parseGeneric
// =============================================================================

func TestDetectProvider_BitbucketHeader(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	store := &pipelineMockStore{}
	recv := NewReceiver(store, nil, events, logger)

	body := `{"push":{"changes":[]}}`
	req := httptest.NewRequest("POST", "/hooks/v1/wh-bb", strings.NewReader(body))
	req.SetPathValue("webhookID", "wh-bb")
	req.Header.Set("X-Event-Key", "repo:push")
	rr := httptest.NewRecorder()
	recv.HandleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestParseGitLab_WithCommitsAndProject(t *testing.T) {
	body := []byte(`{
		"ref":"refs/heads/develop",
		"checkout_sha":"deadbeef",
		"commits":[
			{"message":"first commit","author":{"name":"Dev A"}},
			{"message":"second commit","author":{"name":"Dev B"}}
		],
		"project":{
			"git_http_url":"https://gitlab.com/org/repo.git",
			"path_with_namespace":"org/repo"
		}
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	payload, err := parseGitLab(body, req)
	if err != nil {
		t.Fatalf("parseGitLab: %v", err)
	}
	if payload.Branch != "develop" {
		t.Errorf("branch = %q, want develop", payload.Branch)
	}
	if payload.CommitSHA != "deadbeef" {
		t.Errorf("commit_sha = %q, want deadbeef", payload.CommitSHA)
	}
	if payload.CommitMsg != "second commit" {
		t.Errorf("commit_msg = %q, want 'second commit'", payload.CommitMsg)
	}
	if payload.Author != "Dev B" {
		t.Errorf("author = %q, want 'Dev B'", payload.Author)
	}
}

func TestParseGitea_WithGogsEventHeader(t *testing.T) {
	body := []byte(`{
		"ref":"refs/heads/main",
		"after":"aabbcc",
		"commits":[{"message":"gogs commit"}],
		"repository":{"clone_url":"https://gogs.example.com/repo.git","full_name":"user/repo"}
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	req.Header.Set("X-Gogs-Event", "push")
	payload, err := parseGitea(body, req)
	if err != nil {
		t.Fatalf("parseGitea: %v", err)
	}
	if payload.EventType != "push" {
		t.Errorf("event_type = %q, want push", payload.EventType)
	}
	if payload.CommitSHA != "aabbcc" {
		t.Errorf("commit_sha = %q, want aabbcc", payload.CommitSHA)
	}
}

func TestParseGeneric_CustomProviderJSON(t *testing.T) {
	body := []byte(`{"provider":"custom","event_type":"deploy","branch":"main","commit_sha":"112233"}`)
	payload, err := parseGeneric(body)
	if err != nil {
		t.Fatalf("parseGeneric: %v", err)
	}
	if payload.Provider != "custom" {
		t.Errorf("provider = %q, want custom", payload.Provider)
	}
}

func TestParseGeneric_EmptyProviderDefaultsGeneric(t *testing.T) {
	body := []byte(`{"branch":"main"}`)
	payload, err := parseGeneric(body)
	if err != nil {
		t.Fatalf("parseGeneric: %v", err)
	}
	if payload.Provider != "generic" {
		t.Errorf("provider = %q, want generic", payload.Provider)
	}
}

func TestVerifySignature_GenericAlwaysTrue(t *testing.T) {
	req := httptest.NewRequest("POST", "/hooks/v1/wh", strings.NewReader(""))
	if !VerifySignature(context.Background(), "generic", []byte("data"), "any", req) {
		t.Error("generic should always verify")
	}
}

func TestVerifyGitLabToken_Matches(t *testing.T) {
	if !VerifyGitLabToken("same-token", "same-token") {
		t.Error("matching tokens should pass")
	}
}

func TestVerifyGitLabToken_DoesNotMatch(t *testing.T) {
	if VerifyGitLabToken("wrong-token", "correct-token") {
		t.Error("mismatched tokens should fail")
	}
}

// =============================================================================
// Minimal mock store for pipeline tests
// =============================================================================

type pipelineMockStore struct {
	getAppErr error
}

func (m *pipelineMockStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	if m.getAppErr != nil {
		return nil, m.getAppErr
	}
	return &core.Application{ID: "app-1", Name: "test-app", TenantID: "t1"}, nil
}
func (m *pipelineMockStore) UpdateAppStatus(_ context.Context, _, _ string) error { return nil }
func (m *pipelineMockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (m *pipelineMockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error {
	return nil
}
func (m *pipelineMockStore) Close() error                 { return nil }
func (m *pipelineMockStore) Ping(_ context.Context) error { return nil }
func (m *pipelineMockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) CreateUser(_ context.Context, _ *core.User) error    { return nil }
func (m *pipelineMockStore) UpdateUser(_ context.Context, _ *core.User) error    { return nil }
func (m *pipelineMockStore) UpdatePassword(_ context.Context, _, _ string) error { return nil }
func (m *pipelineMockStore) UpdateLastLogin(_ context.Context, _ string) error   { return nil }
func (m *pipelineMockStore) CountUsers(_ context.Context) (int, error)           { return 0, nil }
func (m *pipelineMockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *pipelineMockStore) CreateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (m *pipelineMockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (m *pipelineMockStore) DeleteTenant(_ context.Context, _ string) error       { return nil }
func (m *pipelineMockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *pipelineMockStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *pipelineMockStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *pipelineMockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (m *pipelineMockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (m *pipelineMockStore) DeleteApp(_ context.Context, _ string) error          { return nil }
func (m *pipelineMockStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (m *pipelineMockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (m *pipelineMockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	return nil, nil
}
func (m *pipelineMockStore) DeleteDomain(_ context.Context, _ string) error { return nil }
func (m *pipelineMockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (m *pipelineMockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}
func (m *pipelineMockStore) CreateProject(_ context.Context, _ *core.Project) error { return nil }
func (m *pipelineMockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (m *pipelineMockStore) DeleteProject(_ context.Context, _ string) error            { return nil }
func (m *pipelineMockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (m *pipelineMockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *pipelineMockStore) CreateSecret(_ context.Context, _ *core.Secret) error { return nil }
func (m *pipelineMockStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}
func (m *pipelineMockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (m *pipelineMockStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return nil, nil
}
func (m *pipelineMockStore) UpdateSecretVersionValue(_ context.Context, _, _ string) error {
	return nil
}
func (m *pipelineMockStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, core.ErrNotFound
}
func (m *pipelineMockStore) CreateInvite(_ context.Context, _ *core.Invitation) error { return nil }
func (m *pipelineMockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (m *pipelineMockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}
func (m *pipelineMockStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (m *pipelineMockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) { return nil, 0, nil }
func (m *pipelineMockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *pipelineMockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) { return nil, 0, nil }
func (m *pipelineMockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error { return nil }

// pipelineMockRuntime implements core.ContainerRuntime.
type pipelineMockRuntime struct{}

func (m *pipelineMockRuntime) Ping() error { return nil }
func (m *pipelineMockRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "container-id", nil
}
func (m *pipelineMockRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *pipelineMockRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *pipelineMockRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *pipelineMockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *pipelineMockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (m *pipelineMockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *pipelineMockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *pipelineMockRuntime) ImagePull(_ context.Context, _ string) error           { return nil }
func (m *pipelineMockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (m *pipelineMockRuntime) ImageRemove(_ context.Context, _ string) error         { return nil }
func (m *pipelineMockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *pipelineMockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}
