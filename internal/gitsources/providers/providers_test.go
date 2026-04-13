package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =========================================================================
// Registry tests
// =========================================================================

func TestRegistryContainsAllProviders(t *testing.T) {
	expected := []string{"github", "gitlab", "gitea", "bitbucket"}
	for _, name := range expected {
		if _, ok := Registry[name]; !ok {
			t.Errorf("Registry missing provider %q", name)
		}
	}
	if len(Registry) != 4 {
		t.Errorf("Registry has %d entries, want 4", len(Registry))
	}
}

func TestRegistryFactoriesReturnNonNil(t *testing.T) {
	for name, factory := range Registry {
		provider := factory("test-token")
		if provider == nil {
			t.Errorf("Factory for %q returned nil", name)
		}
	}
}

// =========================================================================
// GitHub provider tests
// =========================================================================

func TestGitHubName(t *testing.T) {
	p := NewGitHub("tok")
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestGitHubImplementsGitProvider(t *testing.T) {
	var _ core.GitProvider = (*GitHub)(nil)
}

func TestGitHubListRepos_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user/repos") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify auth header
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"full_name":      "owner/repo1",
				"clone_url":      "https://github.com/owner/repo1.git",
				"ssh_url":        "git@github.com:owner/repo1.git",
				"description":    "Test repo",
				"default_branch": "main",
				"private":        false,
			},
			{
				"full_name":      "owner/repo2",
				"clone_url":      "https://github.com/owner/repo2.git",
				"ssh_url":        "git@github.com:owner/repo2.git",
				"description":    "Private repo",
				"default_branch": "master",
				"private":        true,
			},
		})
	}))
	defer srv.Close()

	g := &GitHub{
		token:  "test-token",
		client: srv.Client(),
	}
	// Override the base URL by replacing the do method's URL
	// We need to use the test server URL. Since do() hardcodes "https://api.github.com",
	// we override the client's transport to redirect.
	g.client = newRedirectClient(srv)

	repos, err := g.ListRepos(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("ListRepos() returned %d repos, want 2", len(repos))
	}
	if repos[0].FullName != "owner/repo1" {
		t.Errorf("repos[0].FullName = %q, want %q", repos[0].FullName, "owner/repo1")
	}
	if repos[0].CloneURL != "https://github.com/owner/repo1.git" {
		t.Errorf("repos[0].CloneURL = %q", repos[0].CloneURL)
	}
	if repos[0].SSHURL != "git@github.com:owner/repo1.git" {
		t.Errorf("repos[0].SSHURL = %q", repos[0].SSHURL)
	}
	if repos[0].DefaultBranch != "main" {
		t.Errorf("repos[0].DefaultBranch = %q", repos[0].DefaultBranch)
	}
	if repos[0].Private {
		t.Error("repos[0] should not be private")
	}
	if !repos[1].Private {
		t.Error("repos[1] should be private")
	}
}

func TestGitHubListRepos_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer srv.Close()

	g := &GitHub{token: "bad-token", client: newRedirectClient(srv)}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %q, want HTTP 403 mention", err.Error())
	}
}

func TestGitHubListBranches_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repos/owner/repo/branches") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "main"},
			{"name": "develop"},
			{"name": "feature/x"},
		})
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	branches, err := g.ListBranches(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3", len(branches))
	}
	if branches[0] != "main" {
		t.Errorf("branches[0] = %q", branches[0])
	}
}

func TestGitHubListBranches_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	_, err := g.ListBranches(context.Background(), "nonexistent/repo")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestGitHubGetRepoInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"full_name":      "owner/repo",
			"clone_url":      "https://github.com/owner/repo.git",
			"ssh_url":        "git@github.com:owner/repo.git",
			"description":    "A cool repo",
			"default_branch": "main",
			"private":        true,
		})
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	repo, err := g.GetRepoInfo(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("GetRepoInfo() error = %v", err)
	}
	if repo.FullName != "owner/repo" {
		t.Errorf("FullName = %q", repo.FullName)
	}
	if repo.Description != "A cool repo" {
		t.Errorf("Description = %q", repo.Description)
	}
	if !repo.Private {
		t.Error("expected private = true")
	}
}

func TestGitHubGetRepoInfo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	_, err := g.GetRepoInfo(context.Background(), "owner/repo")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestGitHubCreateWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/repos/owner/repo/hooks") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify request body
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["config"]; !ok {
			t.Error("missing config in payload")
		}
		if _, ok := body["events"]; !ok {
			t.Error("missing events in payload")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 42})
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	id, err := g.CreateWebhook(context.Background(), "owner/repo", "https://example.com/hook", "secret123", []string{"push", "pull_request"})
	if err != nil {
		t.Fatalf("CreateWebhook() error = %v", err)
	}
	if id != "42" {
		t.Errorf("webhook ID = %q, want %q", id, "42")
	}
}

func TestGitHubCreateWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	_, err := g.CreateWebhook(context.Background(), "owner/repo", "https://example.com/hook", "secret", []string{"push"})
	if err == nil {
		t.Fatal("expected error for HTTP 422")
	}
}

func TestGitHubDeleteWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/repos/owner/repo/hooks/42") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	err := g.DeleteWebhook(context.Background(), "owner/repo", "42")
	if err != nil {
		t.Errorf("DeleteWebhook() error = %v", err)
	}
}

func TestGitHubDeleteWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	err := g.DeleteWebhook(context.Background(), "owner/repo", "999")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestGitHubNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %q", auth)
		}
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	g := &GitHub{token: "", client: newRedirectClient(srv)}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ListRepos() with no token error = %v", err)
	}
}

// =========================================================================
// GitLab provider tests
// =========================================================================

func TestGitLabName(t *testing.T) {
	p := NewGitLab("tok")
	if p.Name() != "gitlab" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitlab")
	}
}

func TestGitLabImplementsGitProvider(t *testing.T) {
	var _ core.GitProvider = (*GitLab)(nil)
}

func TestGitLabListRepos_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/projects") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if tok := r.Header.Get("PRIVATE-TOKEN"); tok != "gl-token" {
			t.Errorf("PRIVATE-TOKEN = %q", tok)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"path_with_namespace": "group/project1",
				"http_url_to_repo":    "https://gitlab.com/group/project1.git",
				"ssh_url_to_repo":     "git@gitlab.com:group/project1.git",
				"description":         "GitLab project",
				"default_branch":      "main",
				"visibility":          "private",
			},
			{
				"path_with_namespace": "group/project2",
				"http_url_to_repo":    "https://gitlab.com/group/project2.git",
				"ssh_url_to_repo":     "git@gitlab.com:group/project2.git",
				"description":         "Public project",
				"default_branch":      "master",
				"visibility":          "public",
			},
		})
	}))
	defer srv.Close()

	g := &GitLab{token: "gl-token", baseURL: srv.URL, client: srv.Client()}
	repos, err := g.ListRepos(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}
	if repos[0].FullName != "group/project1" {
		t.Errorf("FullName = %q", repos[0].FullName)
	}
	if !repos[0].Private {
		t.Error("repos[0] should be private (visibility != public)")
	}
	if repos[1].Private {
		t.Error("repos[1] should NOT be private (visibility = public)")
	}
}

func TestGitLabListRepos_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	g := &GitLab{token: "bad", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGitLabListBranches_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "main"},
			{"name": "dev"},
		})
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	branches, err := g.ListBranches(context.Background(), "group%2Fproject")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("got %d branches, want 2", len(branches))
	}
	if branches[0] != "main" {
		t.Errorf("branches[0] = %q", branches[0])
	}
}

func TestGitLabListBranches_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListBranches(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitLabGetRepoInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"path_with_namespace": "group/project",
			"http_url_to_repo":    "https://gitlab.com/group/project.git",
			"ssh_url_to_repo":     "git@gitlab.com:group/project.git",
			"description":         "desc",
			"default_branch":      "main",
		})
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	repo, err := g.GetRepoInfo(context.Background(), "group%2Fproject")
	if err != nil {
		t.Fatalf("GetRepoInfo() error = %v", err)
	}
	if repo.FullName != "group/project" {
		t.Errorf("FullName = %q", repo.FullName)
	}
	if repo.CloneURL != "https://gitlab.com/group/project.git" {
		t.Errorf("CloneURL = %q", repo.CloneURL)
	}
}

func TestGitLabGetRepoInfo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.GetRepoInfo(context.Background(), "group%2Fproject")
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
}

func TestGitLabCreateWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["url"] == nil {
			t.Error("missing url in payload")
		}
		if body["token"] == nil {
			t.Error("missing token (secret) in payload")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 77})
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	id, err := g.CreateWebhook(context.Background(), "group%2Fproject", "https://example.com/hook", "secret", []string{"push"})
	if err != nil {
		t.Fatalf("CreateWebhook() error = %v", err)
	}
	if id != "77" {
		t.Errorf("webhook ID = %q, want %q", id, "77")
	}
}

func TestGitLabCreateWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.CreateWebhook(context.Background(), "group%2Fproject", "url", "secret", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
}

func TestGitLabDeleteWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	err := g.DeleteWebhook(context.Background(), "group%2Fproject", "77")
	if err != nil {
		t.Errorf("DeleteWebhook() error = %v", err)
	}
}

func TestGitLabDeleteWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	err := g.DeleteWebhook(context.Background(), "group%2Fproject", "999")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

// =========================================================================
// Gitea provider tests
// =========================================================================

func TestGiteaName(t *testing.T) {
	p := NewGitea("tok")
	if p.Name() != "gitea" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitea")
	}
}

func TestGiteaImplementsGitProvider(t *testing.T) {
	var _ core.GitProvider = (*Gitea)(nil)
}

func TestGiteaListRepos_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/user/repos") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "token gitea-tok" {
			t.Errorf("Authorization = %q", auth)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"full_name":      "owner/repo",
				"clone_url":      "https://gitea.com/owner/repo.git",
				"ssh_url":        "git@gitea.com:owner/repo.git",
				"description":    "Gitea repo",
				"default_branch": "main",
				"private":        false,
			},
		})
	}))
	defer srv.Close()

	g := &Gitea{token: "gitea-tok", baseURL: srv.URL, client: srv.Client()}
	repos, err := g.ListRepos(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(repos))
	}
	if repos[0].FullName != "owner/repo" {
		t.Errorf("FullName = %q", repos[0].FullName)
	}
	if repos[0].Private {
		t.Error("expected private = false")
	}
}

func TestGiteaListRepos_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	g := &Gitea{token: "bad", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGiteaListBranches_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "main"},
			{"name": "staging"},
		})
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	branches, err := g.ListBranches(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("got %d branches, want 2", len(branches))
	}
}

func TestGiteaListBranches_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListBranches(context.Background(), "nope/nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGiteaGetRepoInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"full_name":      "owner/repo",
			"clone_url":      "https://gitea.com/owner/repo.git",
			"ssh_url":        "git@gitea.com:owner/repo.git",
			"description":    "Desc",
			"default_branch": "main",
			"private":        true,
		})
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	repo, err := g.GetRepoInfo(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("GetRepoInfo() error = %v", err)
	}
	if repo.FullName != "owner/repo" {
		t.Errorf("FullName = %q", repo.FullName)
	}
	if !repo.Private {
		t.Error("expected private = true")
	}
}

func TestGiteaGetRepoInfo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.GetRepoInfo(context.Background(), "owner/repo")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestGiteaCreateWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["type"] != "gitea" {
			t.Errorf("type = %v, want gitea", body["type"])
		}
		if _, ok := body["config"]; !ok {
			t.Error("missing config")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 99})
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	id, err := g.CreateWebhook(context.Background(), "owner/repo", "https://example.com/hook", "secret", []string{"push"})
	if err != nil {
		t.Fatalf("CreateWebhook() error = %v", err)
	}
	if id != "99" {
		t.Errorf("webhook ID = %q, want %q", id, "99")
	}
}

func TestGiteaCreateWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.CreateWebhook(context.Background(), "owner/repo", "url", "secret", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGiteaDeleteWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	err := g.DeleteWebhook(context.Background(), "owner/repo", "99")
	if err != nil {
		t.Errorf("DeleteWebhook() error = %v", err)
	}
}

func TestGiteaDeleteWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	err := g.DeleteWebhook(context.Background(), "owner/repo", "999")
	if err == nil {
		t.Fatal("expected error")
	}
}

// =========================================================================
// Bitbucket provider tests
// =========================================================================

func TestBitbucketName(t *testing.T) {
	p := NewBitbucket("tok")
	if p.Name() != "bitbucket" {
		t.Errorf("Name() = %q, want %q", p.Name(), "bitbucket")
	}
}

func TestBitbucketImplementsGitProvider(t *testing.T) {
	var _ core.GitProvider = (*Bitbucket)(nil)
}

func TestBitbucketListRepos_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repositories") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer bb-token" {
			t.Errorf("Authorization = %q", auth)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{
				{
					"full_name": "team/repo1",
					"links": map[string]any{
						"clone": []map[string]string{
							{"href": "https://bitbucket.org/team/repo1.git", "name": "https"},
							{"href": "git@bitbucket.org:team/repo1.git", "name": "ssh"},
						},
					},
					"description": "BB repo",
					"mainbranch":  map[string]string{"name": "main"},
					"is_private":  true,
				},
				{
					"full_name": "team/repo2",
					"links": map[string]any{
						"clone": []map[string]string{
							{"href": "https://bitbucket.org/team/repo2.git", "name": "https"},
						},
					},
					"description": "Public BB repo",
					"mainbranch":  map[string]string{"name": "master"},
					"is_private":  false,
				},
			},
		})
	}))
	defer srv.Close()

	b := &Bitbucket{token: "bb-token", client: newRedirectClient(srv)}
	repos, err := b.ListRepos(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}
	if repos[0].FullName != "team/repo1" {
		t.Errorf("FullName = %q", repos[0].FullName)
	}
	if repos[0].CloneURL != "https://bitbucket.org/team/repo1.git" {
		t.Errorf("CloneURL = %q", repos[0].CloneURL)
	}
	if repos[0].SSHURL != "git@bitbucket.org:team/repo1.git" {
		t.Errorf("SSHURL = %q", repos[0].SSHURL)
	}
	if !repos[0].Private {
		t.Error("repos[0] should be private")
	}
	if repos[1].Private {
		t.Error("repos[1] should not be private")
	}
	if repos[0].DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q", repos[0].DefaultBranch)
	}
}

func TestBitbucketListRepos_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	b := &Bitbucket{token: "bad", client: newRedirectClient(srv)}
	_, err := b.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
}

func TestBitbucketListBranches_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]string{
				{"name": "main"},
				{"name": "release/1.0"},
			},
		})
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	branches, err := b.ListBranches(context.Background(), "team/repo")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("got %d branches, want 2", len(branches))
	}
	if branches[0] != "main" {
		t.Errorf("branches[0] = %q", branches[0])
	}
	if branches[1] != "release/1.0" {
		t.Errorf("branches[1] = %q", branches[1])
	}
}

func TestBitbucketListBranches_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	_, err := b.ListBranches(context.Background(), "nope/repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBitbucketGetRepoInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"full_name":   "team/repo",
			"description": "BB repo",
			"mainbranch":  map[string]string{"name": "main"},
			"is_private":  true,
		})
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	repo, err := b.GetRepoInfo(context.Background(), "team/repo")
	if err != nil {
		t.Fatalf("GetRepoInfo() error = %v", err)
	}
	if repo.FullName != "team/repo" {
		t.Errorf("FullName = %q", repo.FullName)
	}
	if !repo.Private {
		t.Error("expected private = true")
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q", repo.DefaultBranch)
	}
}

func TestBitbucketGetRepoInfo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	_, err := b.GetRepoInfo(context.Background(), "team/repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBitbucketCreateWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["description"] != "DeployMonster webhook" {
			t.Errorf("description = %v", body["description"])
		}
		events, ok := body["events"].([]any)
		if !ok {
			t.Fatal("events not an array")
		}
		// Verify event name mapping: "push" -> "repo:push"
		found := false
		for _, e := range events {
			if e == "repo:push" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected repo:push in events, got %v", events)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"uuid": "{abc-123-def}"})
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	id, err := b.CreateWebhook(context.Background(), "team/repo", "https://example.com/hook", "secret", []string{"push", "pull_request"})
	if err != nil {
		t.Fatalf("CreateWebhook() error = %v", err)
	}
	// Bitbucket returns UUID
	if id != "{abc-123-def}" {
		t.Errorf("webhook ID = %q, want %q", id, "{abc-123-def}")
	}
}

func TestBitbucketCreateWebhook_EventMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"push", "repo:push"},
		{"pull_request", "pullrequest:created"},
		{"custom_event", "custom_event"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				events := body["events"].([]any)
				if len(events) != 1 || events[0] != tt.expected {
					t.Errorf("events = %v, want [%s]", events, tt.expected)
				}
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]string{"uuid": "u"})
			}))
			defer srv.Close()

			b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
			_, err := b.CreateWebhook(context.Background(), "team/repo", "url", "secret", []string{tt.input})
			if err != nil {
				t.Fatalf("CreateWebhook() error = %v", err)
			}
		})
	}
}

func TestBitbucketCreateWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	_, err := b.CreateWebhook(context.Background(), "team/repo", "url", "secret", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBitbucketDeleteWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	err := b.DeleteWebhook(context.Background(), "team/repo", "{abc-123}")
	if err != nil {
		t.Errorf("DeleteWebhook() error = %v", err)
	}
}

func TestBitbucketDeleteWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	err := b.DeleteWebhook(context.Background(), "team/repo", "{nope}")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestBitbucketCloneURLExtraction verifies the special clone URL extraction from links.
func TestBitbucketCloneURLExtraction(t *testing.T) {
	tests := []struct {
		name      string
		cloneData []map[string]string
		wantHTTPS string
		wantSSH   string
	}{
		{
			name:      "both https and ssh",
			cloneData: []map[string]string{{"href": "https://bb.org/repo.git", "name": "https"}, {"href": "git@bb.org:repo.git", "name": "ssh"}},
			wantHTTPS: "https://bb.org/repo.git",
			wantSSH:   "git@bb.org:repo.git",
		},
		{
			name:      "only https",
			cloneData: []map[string]string{{"href": "https://bb.org/repo.git", "name": "https"}},
			wantHTTPS: "https://bb.org/repo.git",
			wantSSH:   "",
		},
		{
			name:      "only ssh",
			cloneData: []map[string]string{{"href": "git@bb.org:repo.git", "name": "ssh"}},
			wantHTTPS: "",
			wantSSH:   "git@bb.org:repo.git",
		},
		{
			name:      "no clone links",
			cloneData: []map[string]string{},
			wantHTTPS: "",
			wantSSH:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"values": []map[string]any{
						{
							"full_name": "team/repo",
							"links": map[string]any{
								"clone": tt.cloneData,
							},
							"description": "",
							"mainbranch":  map[string]string{"name": "main"},
							"is_private":  false,
						},
					},
				})
			}))
			defer srv.Close()

			b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
			repos, err := b.ListRepos(context.Background(), 1, 10)
			if err != nil {
				t.Fatalf("ListRepos() error = %v", err)
			}
			if len(repos) != 1 {
				t.Fatalf("got %d repos, want 1", len(repos))
			}
			if repos[0].CloneURL != tt.wantHTTPS {
				t.Errorf("CloneURL = %q, want %q", repos[0].CloneURL, tt.wantHTTPS)
			}
			if repos[0].SSHURL != tt.wantSSH {
				t.Errorf("SSHURL = %q, want %q", repos[0].SSHURL, tt.wantSSH)
			}
		})
	}
}

// =========================================================================
// Test do() error paths (request creation failure, network error)
// =========================================================================

func TestGitHub_DoNetworkError(t *testing.T) {
	// Use a client pointing to a closed server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // Close immediately

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "github API") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGitLab_DoNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "gitlab API") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGitea_DoNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "gitea API") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestBitbucket_DoNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	_, err := b.ListRepos(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "bitbucket API") {
		t.Errorf("error = %q", err.Error())
	}
}

// Test with canceled context to trigger NewRequestWithContext error or client.Do error
func TestGitHub_DoCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	g := &GitHub{token: "tok", client: newRedirectClient(srv)}
	_, err := g.ListRepos(ctx, 1, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGitLab_DoCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := &GitLab{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListRepos(ctx, 1, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGitea_DoCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := &Gitea{token: "tok", baseURL: srv.URL, client: srv.Client()}
	_, err := g.ListRepos(ctx, 1, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestBitbucket_DoCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"values": []any{}})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	b := &Bitbucket{token: "tok", client: newRedirectClient(srv)}
	_, err := b.ListRepos(ctx, 1, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

// =========================================================================
// Helpers: redirect client for providers that hardcode base URLs
// =========================================================================

// newRedirectClient returns an http.Client that rewrites any outgoing request
// URL to point to the given test server, preserving path and query.
func newRedirectClient(ts *httptest.Server) *http.Client {
	return &http.Client{
		Transport: &rewriteTransport{target: ts.URL, base: ts.Client().Transport},
	}
}

type rewriteTransport struct {
	target string
	base   http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite URL to point to test server
	newURL := rt.target + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, fmt.Errorf("rewrite request: %w", err)
	}
	newReq.Header = req.Header
	transport := rt.base
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(newReq)
}
