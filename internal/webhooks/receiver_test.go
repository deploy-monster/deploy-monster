package webhooks

import (
	"net/http"
	"testing"
)

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

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "mysecret"

	// Compute correct signature
	sig := "sha256=" + computeHMACSHA256(body, secret)

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

func TestVerifyGitLabToken(t *testing.T) {
	if !VerifyGitLabToken("my-token", "my-token") {
		t.Error("matching token should pass")
	}
	if VerifyGitLabToken("my-token", "wrong-token") {
		t.Error("wrong token should fail")
	}
}

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
	if payload.Branch != "main" {
		t.Errorf("branch = %q, want main", payload.Branch)
	}
	if payload.CommitSHA != "abc123def456" {
		t.Errorf("commit = %q, want abc123def456", payload.CommitSHA)
	}
	if payload.Author != "Ersin" {
		t.Errorf("author = %q, want Ersin", payload.Author)
	}
	if payload.RepoName != "deploy-monster/app" {
		t.Errorf("repo = %q, want deploy-monster/app", payload.RepoName)
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

	if payload.Branch != "develop" {
		t.Errorf("branch = %q, want develop", payload.Branch)
	}
	if payload.CommitSHA != "def789" {
		t.Errorf("commit = %q, want def789", payload.CommitSHA)
	}
}

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
