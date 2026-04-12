package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// --- Provider Detection ---

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"github", map[string]string{"X-GitHub-Event": "push"}, "github"},
		{"gitlab", map[string]string{"X-Gitlab-Event": "Push Hook"}, "gitlab"},
		{"gitea", map[string]string{"X-Gitea-Event": "push"}, "gitea"},
		{"gogs", map[string]string{"X-Gogs-Event": "push"}, "gogs"},
		{"bitbucket", map[string]string{"X-Event-Key": "repo:push"}, "bitbucket"},
		{"generic", map[string]string{}, "generic"},
		{"generic_unknown_header", map[string]string{"X-Custom": "something"}, "generic"},
		{"github_push", map[string]string{"X-GitHub-Event": "push"}, "github"},
		{"github_ping", map[string]string{"X-GitHub-Event": "ping"}, "github"},
		{"github_pr", map[string]string{"X-GitHub-Event": "pull_request"}, "github"},
		{"gitlab_mr", map[string]string{"X-Gitlab-Event": "Merge Request Hook"}, "gitlab"},
		{"gitea_create", map[string]string{"X-Gitea-Event": "create"}, "gitea"},
		{"bitbucket_pr", map[string]string{"X-Event-Key": "pullrequest:created"}, "bitbucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			got := detectProvider(r)
			if got != tt.want {
				t.Errorf("detectProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- GitHub Signature Verification ---

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "mysecret"

	// Compute correct signature.
	sig := "sha256=" + signPayload(body, secret)

	if !VerifyGitHubSignature(body, secret, sig) {
		t.Error("valid signature should pass")
	}

	if VerifyGitHubSignature(body, "wrong-secret", sig) {
		t.Error("wrong secret should fail")
	}

	if VerifyGitHubSignature(body, secret, "sha256=invalid") {
		t.Error("invalid signature should fail")
	}

	if VerifyGitHubSignature(body, secret, "not-sha256") {
		t.Error("missing sha256= prefix should fail")
	}
}

func TestVerifyGitHubSignature_TableDriven(t *testing.T) {
	body := []byte(`{"action":"push"}`)
	secret := "test-secret-key"
	correctSig := "sha256=" + signPayload(body, secret)

	tests := []struct {
		name      string
		body      []byte
		secret    string
		signature string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      body,
			secret:    secret,
			signature: correctSig,
			want:      true,
		},
		{
			name:      "wrong secret",
			body:      body,
			secret:    "wrong-secret",
			signature: correctSig,
			want:      false,
		},
		{
			name:      "tampered body",
			body:      []byte(`{"action":"tampered"}`),
			secret:    secret,
			signature: correctSig,
			want:      false,
		},
		{
			name:      "missing sha256 prefix",
			body:      body,
			secret:    secret,
			signature: signPayload(body, secret),
			want:      false,
		},
		{
			name:      "empty signature",
			body:      body,
			secret:    secret,
			signature: "",
			want:      false,
		},
		{
			name:      "empty secret",
			body:      body,
			secret:    "",
			signature: "sha256=" + signPayload(body, ""),
			want:      true,
		},
		{
			name:      "sha256= only",
			body:      body,
			secret:    secret,
			signature: "sha256=",
			want:      false,
		},
		{
			name:      "invalid hex chars",
			body:      body,
			secret:    secret,
			signature: "sha256=xyz_not_hex",
			want:      false,
		},
		{
			name:      "empty body valid signature",
			body:      []byte{},
			secret:    secret,
			signature: "sha256=" + signPayload([]byte{}, secret),
			want:      true,
		},
		{
			name:      "large body",
			body:      bytes.Repeat([]byte("a"), 10000),
			secret:    secret,
			signature: "sha256=" + signPayload(bytes.Repeat([]byte("a"), 10000), secret),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyGitHubSignature(tt.body, tt.secret, tt.signature)
			if got != tt.want {
				t.Errorf("VerifyGitHubSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- GitLab Token Verification ---

func TestVerifyGitLabToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		secret string
		want   bool
	}{
		{"matching token", "my-token", "my-token", true},
		{"wrong token", "my-token", "wrong-token", false},
		{"empty header", "", "secret", false},
		{"empty secret", "token", "", false},
		{"both empty", "", "", true},
		{"long token match", "a-very-long-token-123-456-789", "a-very-long-token-123-456-789", true},
		{"case sensitive mismatch", "MyToken", "mytoken", false},
		{"unicode tokens", "tok\u00e9n", "tok\u00e9n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyGitLabToken(tt.header, tt.secret)
			if got != tt.want {
				t.Errorf("VerifyGitLabToken(%q, %q) = %v, want %v", tt.header, tt.secret, got, tt.want)
			}
		})
	}
}

// --- Gitea/Gogs Signature Verification ---

func TestVerifySignature_Gitea(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "gitea-secret"
	sig := signPayload(body, secret)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Gitea-Signature", sig)
	r.Header.Set("X-Gitea-Event", "push")

	if !VerifySignature(context.Background(), "gitea", body, secret, r) {
		t.Error("valid Gitea signature should pass")
	}

	// Wrong secret should fail.
	if VerifySignature(context.Background(), "gitea", body, "wrong", r) {
		t.Error("wrong Gitea secret should fail")
	}
}

func TestVerifySignature_Gogs(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/develop"}`)
	secret := "gogs-secret"
	sig := signPayload(body, secret)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Gogs-Signature", sig)
	r.Header.Set("X-Gogs-Event", "push")

	if !VerifySignature(context.Background(), "gogs", body, secret, r) {
		t.Error("valid Gogs signature should pass")
	}
}

func TestVerifySignature_GiteaFallsBackToGogs(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "test-secret"
	sig := signPayload(body, secret)

	// Gitea provider but only X-Gogs-Signature set (no X-Gitea-Signature).
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Gogs-Signature", sig)

	if !VerifySignature(context.Background(), "gitea", body, secret, r) {
		t.Error("Gitea should fall back to X-Gogs-Signature header")
	}
}

// --- Bitbucket Verification ---

func TestVerifySignature_Bitbucket(t *testing.T) {
	// Bitbucket uses X-Event-Key header for provider detection but
	// currently has no signature verification in the codebase
	// (falls through to "default" which returns true).
	body := []byte(`{"push":{"changes":[]}}`)
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Event-Key", "repo:push")

	if !VerifySignature(context.Background(), "bitbucket", body, "any-secret", r) {
		t.Error("bitbucket should pass (no signature verification implemented)")
	}
}

// --- Combined VerifySignature ---

func TestVerifySignature_AllProviders(t *testing.T) {
	body := []byte(`{"action":"test"}`)
	secret := "shared-secret"

	tests := []struct {
		name     string
		provider string
		headers  map[string]string
		want     bool
	}{
		{
			name:     "github valid",
			provider: "github",
			headers:  map[string]string{"X-Hub-Signature-256": "sha256=" + signPayload(body, secret)},
			want:     true,
		},
		{
			name:     "github invalid",
			provider: "github",
			headers:  map[string]string{"X-Hub-Signature-256": "sha256=bad"},
			want:     false,
		},
		{
			name:     "github missing header",
			provider: "github",
			headers:  map[string]string{},
			want:     false,
		},
		{
			name:     "gitlab valid",
			provider: "gitlab",
			headers:  map[string]string{"X-Gitlab-Token": secret},
			want:     true,
		},
		{
			name:     "gitlab invalid",
			provider: "gitlab",
			headers:  map[string]string{"X-Gitlab-Token": "wrong"},
			want:     false,
		},
		{
			name:     "gitlab missing header",
			provider: "gitlab",
			headers:  map[string]string{},
			want:     false,
		},
		{
			name:     "gitea valid",
			provider: "gitea",
			headers:  map[string]string{"X-Gitea-Signature": signPayload(body, secret)},
			want:     true,
		},
		{
			name:     "gitea invalid",
			provider: "gitea",
			headers:  map[string]string{"X-Gitea-Signature": "wrong"},
			want:     false,
		},
		{
			name:     "gogs valid",
			provider: "gogs",
			headers:  map[string]string{"X-Gogs-Signature": signPayload(body, secret)},
			want:     true,
		},
		{
			name:     "generic always true",
			provider: "generic",
			headers:  map[string]string{},
			want:     true,
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			headers:  map[string]string{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			got := VerifySignature(context.Background(), tt.provider, body, secret, r)
			if got != tt.want {
				t.Errorf("VerifySignature(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

// --- Payload Parsing ---

func TestParseGitHub(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/main",
		"head_commit": {
			"id": "abc123def456",
			"message": "fix: bug in login",
			"author": {"name": "Ersin"}
		},
		"repository": {
			"full_name": "deploy-monster/app",
			"clone_url": "https://github.com/deploy-monster/app.git"
		}
	}`)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-GitHub-Event", "push")

	payload, err := parseGitHub(body, r)
	if err != nil {
		t.Fatalf("parseGitHub: %v", err)
	}

	if payload.Provider != "github" {
		t.Errorf("provider = %q, want github", payload.Provider)
	}
	if payload.EventType != "push" {
		t.Errorf("event_type = %q, want push", payload.EventType)
	}
	if payload.Branch != "main" {
		t.Errorf("branch = %q, want main", payload.Branch)
	}
	if payload.CommitSHA != "abc123def456" {
		t.Errorf("commit = %q, want abc123def456", payload.CommitSHA)
	}
	if payload.CommitMsg != "fix: bug in login" {
		t.Errorf("commit_message = %q, want 'fix: bug in login'", payload.CommitMsg)
	}
	if payload.Author != "Ersin" {
		t.Errorf("author = %q, want Ersin", payload.Author)
	}
	if payload.RepoName != "deploy-monster/app" {
		t.Errorf("repo = %q, want deploy-monster/app", payload.RepoName)
	}
	if payload.RepoURL != "https://github.com/deploy-monster/app.git" {
		t.Errorf("repo_url = %q, want https://github.com/deploy-monster/app.git", payload.RepoURL)
	}
}

func TestParseGitHub_TagEvent(t *testing.T) {
	body := []byte(`{
		"ref": "refs/tags/v1.0.0",
		"head_commit": {
			"id": "tag123",
			"message": "release v1.0.0",
			"author": {"name": "Release Bot"}
		},
		"repository": {
			"full_name": "org/repo",
			"clone_url": "https://github.com/org/repo.git"
		}
	}`)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-GitHub-Event", "create")

	payload, err := parseGitHub(body, r)
	if err != nil {
		t.Fatalf("parseGitHub tag: %v", err)
	}

	if payload.EventType != "create" {
		t.Errorf("event_type = %q, want create", payload.EventType)
	}
	// Tags don't have "refs/heads/" prefix so TrimPrefix leaves them as-is.
	if payload.Branch != "refs/tags/v1.0.0" {
		t.Errorf("branch = %q, want refs/tags/v1.0.0", payload.Branch)
	}
}

func TestParseGitHub_MinimalPayload(t *testing.T) {
	body := []byte(`{}`)
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-GitHub-Event", "ping")

	payload, err := parseGitHub(body, r)
	if err != nil {
		t.Fatalf("parseGitHub minimal: %v", err)
	}

	if payload.Provider != "github" {
		t.Errorf("provider = %q, want github", payload.Provider)
	}
	if payload.EventType != "ping" {
		t.Errorf("event_type = %q, want ping", payload.EventType)
	}
}

func TestParseGitHub_InvalidJSON(t *testing.T) {
	_, err := parseGitHub([]byte(`not json`), &http.Request{Header: http.Header{}})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseGitLab(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/develop",
		"checkout_sha": "def789",
		"commits": [
			{"message": "update readme", "author": {"name": "Dev"}}
		],
		"project": {
			"path_with_namespace": "team/project",
			"git_http_url": "https://gitlab.com/team/project.git"
		}
	}`)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Gitlab-Event", "Push Hook")

	payload, err := parseGitLab(body, r)
	if err != nil {
		t.Fatalf("parseGitLab: %v", err)
	}

	if payload.Provider != "gitlab" {
		t.Errorf("provider = %q, want gitlab", payload.Provider)
	}
	if payload.EventType != "Push Hook" {
		t.Errorf("event_type = %q, want 'Push Hook'", payload.EventType)
	}
	if payload.Branch != "develop" {
		t.Errorf("branch = %q, want develop", payload.Branch)
	}
	if payload.CommitSHA != "def789" {
		t.Errorf("commit = %q, want def789", payload.CommitSHA)
	}
	if payload.CommitMsg != "update readme" {
		t.Errorf("commit_message = %q, want 'update readme'", payload.CommitMsg)
	}
	if payload.Author != "Dev" {
		t.Errorf("author = %q, want Dev", payload.Author)
	}
	if payload.RepoName != "team/project" {
		t.Errorf("repo_name = %q, want team/project", payload.RepoName)
	}
	if payload.RepoURL != "https://gitlab.com/team/project.git" {
		t.Errorf("repo_url = %q, want https://gitlab.com/team/project.git", payload.RepoURL)
	}
}

func TestParseGitLab_MultipleCommits(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/feature",
		"checkout_sha": "latest123",
		"commits": [
			{"message": "first commit", "author": {"name": "Alice"}},
			{"message": "second commit", "author": {"name": "Bob"}},
			{"message": "last commit", "author": {"name": "Charlie"}}
		],
		"project": {
			"path_with_namespace": "org/repo",
			"git_http_url": "https://gitlab.com/org/repo.git"
		}
	}`)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Gitlab-Event", "Push Hook")

	payload, err := parseGitLab(body, r)
	if err != nil {
		t.Fatalf("parseGitLab: %v", err)
	}

	// Should use last commit.
	if payload.CommitMsg != "last commit" {
		t.Errorf("commit_message = %q, want 'last commit'", payload.CommitMsg)
	}
	if payload.Author != "Charlie" {
		t.Errorf("author = %q, want Charlie", payload.Author)
	}
}

func TestParseGitLab_InvalidJSON(t *testing.T) {
	_, err := parseGitLab([]byte(`{broken`), &http.Request{Header: http.Header{}})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseGitea(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/main",
		"after": "abc999",
		"commits": [
			{"message": "initial commit"}
		],
		"repository": {
			"full_name": "user/repo",
			"clone_url": "https://gitea.example.com/user/repo.git"
		}
	}`)

	r := &http.Request{Header: http.Header{}}
	r.Header.Set("X-Gitea-Event", "push")

	payload, err := parseGitea(body, r)
	if err != nil {
		t.Fatalf("parseGitea: %v", err)
	}

	if payload.Provider != "gitea" {
		t.Errorf("provider = %q, want gitea", payload.Provider)
	}
	if payload.EventType != "push" {
		t.Errorf("event_type = %q, want push", payload.EventType)
	}
	if payload.Branch != "main" {
		t.Errorf("branch = %q, want main", payload.Branch)
	}
	if payload.CommitSHA != "abc999" {
		t.Errorf("commit = %q, want abc999", payload.CommitSHA)
	}
	if payload.CommitMsg != "initial commit" {
		t.Errorf("commit_message = %q, want 'initial commit'", payload.CommitMsg)
	}
	if payload.RepoName != "user/repo" {
		t.Errorf("repo_name = %q, want user/repo", payload.RepoName)
	}
	if payload.RepoURL != "https://gitea.example.com/user/repo.git" {
		t.Errorf("repo_url = %q, want https://gitea.example.com/user/repo.git", payload.RepoURL)
	}
}

func TestParseGitea_GogsEvent(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/develop",
		"after": "gogs123",
		"commits": [
			{"message": "gogs commit"}
		],
		"repository": {
			"full_name": "gogs/repo",
			"clone_url": "https://gogs.example.com/gogs/repo.git"
		}
	}`)

	r := &http.Request{Header: http.Header{}}
	// No X-Gitea-Event, only X-Gogs-Event.
	r.Header.Set("X-Gogs-Event", "push")

	payload, err := parseGitea(body, r)
	if err != nil {
		t.Fatalf("parseGitea (gogs): %v", err)
	}

	if payload.EventType != "push" {
		t.Errorf("event_type = %q, want push (from Gogs header)", payload.EventType)
	}
}

func TestParseGitea_InvalidJSON(t *testing.T) {
	_, err := parseGitea([]byte(`nope`), &http.Request{Header: http.Header{}})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseGeneric(t *testing.T) {
	body := []byte(`{
		"provider": "custom",
		"event_type": "deploy",
		"branch": "release",
		"commit_sha": "gen123",
		"commit_message": "deploy it",
		"author": "CI",
		"repo_url": "https://custom.com/repo",
		"repo_name": "custom/repo"
	}`)

	payload, err := parseGeneric(body)
	if err != nil {
		t.Fatalf("parseGeneric: %v", err)
	}

	if payload.Provider != "custom" {
		t.Errorf("provider = %q, want custom", payload.Provider)
	}
	if payload.EventType != "deploy" {
		t.Errorf("event_type = %q, want deploy", payload.EventType)
	}
	if payload.Branch != "release" {
		t.Errorf("branch = %q, want release", payload.Branch)
	}
}

func TestParseGeneric_NoProvider(t *testing.T) {
	body := []byte(`{"event_type": "push", "branch": "main"}`)

	payload, err := parseGeneric(body)
	if err != nil {
		t.Fatalf("parseGeneric: %v", err)
	}

	if payload.Provider != "generic" {
		t.Errorf("provider = %q, want generic (default)", payload.Provider)
	}
}

func TestParseGeneric_InvalidJSON(t *testing.T) {
	_, err := parseGeneric([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- parsePayload dispatch ---

func TestParsePayload_Dispatch(t *testing.T) {
	githubBody := []byte(`{
		"ref": "refs/heads/main",
		"head_commit": {"id": "abc", "message": "m", "author": {"name": "A"}},
		"repository": {"full_name": "o/r", "clone_url": "https://gh.com/o/r.git"}
	}`)

	tests := []struct {
		name     string
		provider string
		headers  map[string]string
		body     []byte
		wantProv string
	}{
		{
			name:     "dispatch to github",
			provider: "github",
			headers:  map[string]string{"X-GitHub-Event": "push"},
			body:     githubBody,
			wantProv: "github",
		},
		{
			name:     "dispatch to gitlab",
			provider: "gitlab",
			headers:  map[string]string{"X-Gitlab-Event": "Push Hook"},
			body: []byte(`{
				"ref": "refs/heads/main",
				"checkout_sha": "sha",
				"commits": [{"message": "msg", "author": {"name": "A"}}],
				"project": {"path_with_namespace": "t/p", "git_http_url": "https://gl.com/t/p.git"}
			}`),
			wantProv: "gitlab",
		},
		{
			name:     "dispatch to gitea",
			provider: "gitea",
			headers:  map[string]string{"X-Gitea-Event": "push"},
			body: []byte(`{
				"ref": "refs/heads/main",
				"after": "sha",
				"commits": [{"message": "msg"}],
				"repository": {"full_name": "u/r", "clone_url": "https://gt.com/u/r.git"}
			}`),
			wantProv: "gitea",
		},
		{
			name:     "dispatch to generic",
			provider: "unknown",
			headers:  map[string]string{},
			body:     []byte(`{"provider": "custom", "event_type": "push"}`),
			wantProv: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			payload, err := parsePayload(tt.provider, tt.body, r)
			if err != nil {
				t.Fatalf("parsePayload: %v", err)
			}
			if payload.Provider != tt.wantProv {
				t.Errorf("provider = %q, want %q", payload.Provider, tt.wantProv)
			}
		})
	}
}

// --- Handler Tests with httptest ---

func TestHandleWebhook_MissingWebhookID(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	recv := NewReceiver(nil, nil, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Use a mux to simulate real routing.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	// Request without webhookID in path value.
	body := []byte(`{"ref":"refs/heads/main"}`)
	req := httptest.NewRequest("POST", "/hooks/v1/", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	// Directly call handler which should get empty webhookID from PathValue.
	recv.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if !strings.Contains(w.Body.String(), "missing webhook ID") {
		t.Errorf("body = %q, want 'missing webhook ID'", w.Body.String())
	}
}

func TestHandleWebhook_GitHubPush(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	recv := NewReceiver(nil, nil, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Track received events.
	var receivedEvent core.Event
	events.Subscribe(core.EventWebhookReceived, func(_ context.Context, e core.Event) error {
		receivedEvent = e
		return nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	body := []byte(`{
		"ref": "refs/heads/main",
		"head_commit": {
			"id": "sha123",
			"message": "test commit",
			"author": {"name": "Tester"}
		},
		"repository": {
			"full_name": "org/app",
			"clone_url": "https://github.com/org/app.git"
		}
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh-123", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "received" {
		t.Errorf("status = %q, want received", resp["status"])
	}

	// Verify event was published.
	if receivedEvent.Type != core.EventWebhookReceived {
		t.Errorf("event type = %q, want %q", receivedEvent.Type, core.EventWebhookReceived)
	}

	data, ok := receivedEvent.Data.(core.WebhookEventData)
	if !ok {
		t.Fatal("event data is not WebhookEventData")
	}
	if data.WebhookID != "wh-123" {
		t.Errorf("webhook_id = %q, want wh-123", data.WebhookID)
	}
	if data.Provider != "github" {
		t.Errorf("provider = %q, want github", data.Provider)
	}
	if data.Branch != "main" {
		t.Errorf("branch = %q, want main", data.Branch)
	}
	if data.CommitSHA != "sha123" {
		t.Errorf("commit_sha = %q, want sha123", data.CommitSHA)
	}
}

func TestHandleWebhook_GitLabPush(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	recv := NewReceiver(nil, nil, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	body := []byte(`{
		"ref": "refs/heads/staging",
		"checkout_sha": "gl456",
		"commits": [
			{"message": "deploy staging", "author": {"name": "Alice"}}
		],
		"project": {
			"path_with_namespace": "team/svc",
			"git_http_url": "https://gitlab.com/team/svc.git"
		}
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh-gitlab", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWebhook_InvalidPayload(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	recv := NewReceiver(nil, nil, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	req := httptest.NewRequest("POST", "/hooks/v1/wh-bad", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid payload") {
		t.Errorf("body = %q, want 'invalid payload'", w.Body.String())
	}
}

func TestHandleWebhook_GenericProvider(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	recv := NewReceiver(nil, nil, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	body := []byte(`{
		"provider": "custom-ci",
		"event_type": "deploy",
		"branch": "production",
		"commit_sha": "gen789"
	}`)

	req := httptest.NewRequest("POST", "/hooks/v1/wh-generic", bytes.NewReader(body))
	// No provider-specific headers = generic.
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWebhook_EmptyBody(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	recv := NewReceiver(nil, nil, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)

	req := httptest.NewRequest("POST", "/hooks/v1/wh-empty", bytes.NewReader([]byte{}))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Empty body is invalid JSON, should fail parsing.
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- signPayload tests ---

func TestComputeHMACSHA256(t *testing.T) {
	body := []byte("test payload")
	secret := "secret"

	sig := signPayload(body, secret)

	// Verify it produces correct HMAC-SHA256.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if sig != expected {
		t.Errorf("signPayload = %q, want %q", sig, expected)
	}

	// Should be 64 hex characters (32 bytes).
	if len(sig) != 64 {
		t.Errorf("len(sig) = %d, want 64", len(sig))
	}
}

func TestComputeHMACSHA256_Deterministic(t *testing.T) {
	body := []byte("deterministic test")
	secret := "same-secret"

	sig1 := signPayload(body, secret)
	sig2 := signPayload(body, secret)

	if sig1 != sig2 {
		t.Error("same input should produce same output")
	}
}

func TestComputeHMACSHA256_DifferentSecrets(t *testing.T) {
	body := []byte("same body")

	sig1 := signPayload(body, "secret-a")
	sig2 := signPayload(body, "secret-b")

	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestComputeHMACSHA256_DifferentBodies(t *testing.T) {
	secret := "same-secret"

	sig1 := signPayload([]byte("body-a"), secret)
	sig2 := signPayload([]byte("body-b"), secret)

	if sig1 == sig2 {
		t.Error("different bodies should produce different signatures")
	}
}

// --- SignPayload ---

func TestSignPayload(t *testing.T) {
	payload := []byte("hello world")
	secret := "secret123"

	sig1 := signPayload(payload, secret)
	sig2 := signPayload(payload, secret)

	if sig1 != sig2 {
		t.Error("same payload+secret should produce same signature")
	}

	sig3 := signPayload(payload, "different")
	if sig1 == sig3 {
		t.Error("different secret should produce different signature")
	}
}

// --- WebhookPayload JSON round-trip ---

func TestWebhookPayload_JSONRoundTrip(t *testing.T) {
	original := WebhookPayload{
		Provider:  "github",
		EventType: "push",
		Branch:    "main",
		CommitSHA: "abc123",
		CommitMsg: "test commit",
		Author:    "tester",
		RepoURL:   "https://github.com/org/repo.git",
		RepoName:  "org/repo",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded WebhookPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}
