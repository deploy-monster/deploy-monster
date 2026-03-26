package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Mock Git Provider ───────────────────────────────────────────────────────

type mockGitProvider struct {
	repos     []core.GitRepo
	branches  []string
	errRepos  error
	errBranch error
}

func (m *mockGitProvider) Name() string { return "github" }

func (m *mockGitProvider) ListRepos(_ context.Context, _, _ int) ([]core.GitRepo, error) {
	if m.errRepos != nil {
		return nil, m.errRepos
	}
	return m.repos, nil
}

func (m *mockGitProvider) ListBranches(_ context.Context, _ string) ([]string, error) {
	if m.errBranch != nil {
		return nil, m.errBranch
	}
	return m.branches, nil
}

func (m *mockGitProvider) GetRepoInfo(_ context.Context, _ string) (*core.GitRepo, error) {
	return nil, nil
}

func (m *mockGitProvider) CreateWebhook(_ context.Context, _, _, _ string, _ []string) (string, error) {
	return "", nil
}

func (m *mockGitProvider) DeleteWebhook(_ context.Context, _, _ string) error {
	return nil
}

// ─── List Providers ──────────────────────────────────────────────────────────

func TestGitSourceListProviders_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterGitProvider("github", &mockGitProvider{})
	services.RegisterGitProvider("gitlab", &mockGitProvider{})

	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/providers", nil)
	rr := httptest.NewRecorder()

	handler.ListProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 providers, got %d", len(data))
	}
}

func TestGitSourceListProviders_Empty(t *testing.T) {
	services := core.NewServices()
	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/providers", nil)
	rr := httptest.NewRecorder()

	handler.ListProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected 0 providers, got %d", len(data))
	}
}

// ─── List Repos ──────────────────────────────────────────────────────────────

func TestGitSourceListRepos_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterGitProvider("github", &mockGitProvider{
		repos: []core.GitRepo{
			{FullName: "user/repo1", CloneURL: "https://github.com/user/repo1.git", DefaultBranch: "main"},
			{FullName: "user/repo2", CloneURL: "https://github.com/user/repo2.git", DefaultBranch: "master"},
		},
	})

	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/github/repos", nil)
	req.SetPathValue("provider", "github")
	rr := httptest.NewRecorder()

	handler.ListRepos(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 repos, got %d", len(data))
	}

	page := int(resp["page"].(float64))
	if page != 1 {
		t.Errorf("expected page=1, got %d", page)
	}
}

func TestGitSourceListRepos_Pagination(t *testing.T) {
	services := core.NewServices()
	services.RegisterGitProvider("github", &mockGitProvider{
		repos: []core.GitRepo{{FullName: "user/repo1"}},
	})

	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/github/repos?page=3", nil)
	req.SetPathValue("provider", "github")
	rr := httptest.NewRecorder()

	handler.ListRepos(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	page := int(resp["page"].(float64))
	if page != 3 {
		t.Errorf("expected page=3, got %d", page)
	}
}

func TestGitSourceListRepos_ProviderNotFound(t *testing.T) {
	services := core.NewServices()
	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/bitbucket/repos", nil)
	req.SetPathValue("provider", "bitbucket")
	rr := httptest.NewRecorder()

	handler.ListRepos(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "provider not configured")
}

func TestGitSourceListRepos_ProviderError(t *testing.T) {
	services := core.NewServices()
	services.RegisterGitProvider("github", &mockGitProvider{
		errRepos: errors.New("API rate limited"),
	})

	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/github/repos", nil)
	req.SetPathValue("provider", "github")
	rr := httptest.NewRecorder()

	handler.ListRepos(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── List Branches ───────────────────────────────────────────────────────────

func TestGitSourceListBranches_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterGitProvider("github", &mockGitProvider{
		branches: []string{"main", "develop", "feature/auth"},
	})

	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/github/repos/user-repo/branches", nil)
	req.SetPathValue("provider", "github")
	req.SetPathValue("repo", "user-repo")
	rr := httptest.NewRecorder()

	handler.ListBranches(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 3 {
		t.Errorf("expected 3 branches, got %d", len(data))
	}
}

func TestGitSourceListBranches_ProviderNotFound(t *testing.T) {
	services := core.NewServices()
	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/bitbucket/repos/user-repo/branches", nil)
	req.SetPathValue("provider", "bitbucket")
	req.SetPathValue("repo", "user-repo")
	rr := httptest.NewRecorder()

	handler.ListBranches(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "provider not configured")
}

func TestGitSourceListBranches_ProviderError(t *testing.T) {
	services := core.NewServices()
	services.RegisterGitProvider("github", &mockGitProvider{
		errBranch: errors.New("repo not found"),
	})

	handler := NewGitSourceHandler(services)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/github/repos/user-repo/branches", nil)
	req.SetPathValue("provider", "github")
	req.SetPathValue("repo", "user-repo")
	rr := httptest.NewRecorder()

	handler.ListBranches(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to list branches")
}
