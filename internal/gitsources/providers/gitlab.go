package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

var _ core.GitProvider = (*GitLab)(nil)

// GitLab implements core.GitProvider for GitLab.
type GitLab struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewGitLab(token string) core.GitProvider {
	return &GitLab{
		token:   token,
		baseURL: "https://gitlab.com/api/v4",
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GitLab) Name() string { return "gitlab" }

func (g *GitLab) ListRepos(ctx context.Context, page, perPage int) ([]core.GitRepo, error) {
	body, err := g.get(ctx, fmt.Sprintf("/projects?membership=true&page=%d&per_page=%d&order_by=updated_at", page, perPage))
	if err != nil {
		return nil, err
	}

	var raw []struct {
		PathWithNS    string `json:"path_with_namespace"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		SSHURLToRepo  string `json:"ssh_url_to_repo"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Visibility    string `json:"visibility"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}

	repos := make([]core.GitRepo, len(raw))
	for i, r := range raw {
		repos[i] = core.GitRepo{
			FullName: r.PathWithNS, CloneURL: r.HTTPURLToRepo, SSHURL: r.SSHURLToRepo,
			Description: r.Description, DefaultBranch: r.DefaultBranch, Private: r.Visibility != "public",
		}
	}
	return repos, nil
}

func (g *GitLab) ListBranches(ctx context.Context, repoFullName string) ([]string, error) {
	body, err := g.get(ctx, fmt.Sprintf("/projects/%s/repository/branches?per_page=100", repoFullName))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	branches := make([]string, len(raw))
	for i, b := range raw {
		branches[i] = b.Name
	}
	return branches, nil
}

func (g *GitLab) GetRepoInfo(ctx context.Context, repoFullName string) (*core.GitRepo, error) {
	body, err := g.get(ctx, "/projects/"+repoFullName)
	if err != nil {
		return nil, err
	}
	var r struct {
		PathWithNS    string `json:"path_with_namespace"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		SSHURLToRepo  string `json:"ssh_url_to_repo"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return &core.GitRepo{
		FullName: r.PathWithNS, CloneURL: r.HTTPURLToRepo, SSHURL: r.SSHURLToRepo,
		Description: r.Description, DefaultBranch: r.DefaultBranch,
	}, nil
}

func (g *GitLab) CreateWebhook(ctx context.Context, repoFullName, url, secret string, events []string) (string, error) {
	payload := map[string]any{
		"url":                   url,
		"token":                 secret,
		"push_events":           true,
		"tag_push_events":       true,
		"merge_requests_events": true,
	}
	body, err := g.post(ctx, fmt.Sprintf("/projects/%s/hooks", repoFullName), payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}
	return fmt.Sprintf("%d", resp.ID), nil
}

func (g *GitLab) DeleteWebhook(ctx context.Context, repoFullName, webhookID string) error {
	_, err := g.do(ctx, http.MethodDelete, fmt.Sprintf("/projects/%s/hooks/%s", repoFullName, webhookID), nil)
	return err
}

func (g *GitLab) get(ctx context.Context, path string) ([]byte, error) {
	return g.do(ctx, http.MethodGet, path, nil)
}

func (g *GitLab) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return g.do(ctx, http.MethodPost, path, payload)
}

func (g *GitLab) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", g.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gitlab API %s: HTTP %d", path, resp.StatusCode)
	}
	return respBody, nil
}
