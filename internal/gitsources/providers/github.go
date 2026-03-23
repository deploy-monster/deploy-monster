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

// Compile-time check.
var _ core.GitProvider = (*GitHub)(nil)

// GitHub implements core.GitProvider for GitHub.
type GitHub struct {
	token  string
	client *http.Client
}

func NewGitHub(token string) core.GitProvider {
	return &GitHub{
		token:  token,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) ListRepos(ctx context.Context, page, perPage int) ([]core.GitRepo, error) {
	body, err := g.get(ctx, fmt.Sprintf("/user/repos?page=%d&per_page=%d&sort=updated", page, perPage))
	if err != nil {
		return nil, err
	}

	var raw []struct {
		FullName      string `json:"full_name"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
	}
	json.Unmarshal(body, &raw)

	repos := make([]core.GitRepo, len(raw))
	for i, r := range raw {
		repos[i] = core.GitRepo{
			FullName: r.FullName, CloneURL: r.CloneURL, SSHURL: r.SSHURL,
			Description: r.Description, DefaultBranch: r.DefaultBranch, Private: r.Private,
		}
	}
	return repos, nil
}

func (g *GitHub) ListBranches(ctx context.Context, repoFullName string) ([]string, error) {
	body, err := g.get(ctx, fmt.Sprintf("/repos/%s/branches?per_page=100", repoFullName))
	if err != nil {
		return nil, err
	}

	var raw []struct{ Name string `json:"name"` }
	json.Unmarshal(body, &raw)

	branches := make([]string, len(raw))
	for i, b := range raw {
		branches[i] = b.Name
	}
	return branches, nil
}

func (g *GitHub) GetRepoInfo(ctx context.Context, repoFullName string) (*core.GitRepo, error) {
	body, err := g.get(ctx, "/repos/"+repoFullName)
	if err != nil {
		return nil, err
	}

	var r struct {
		FullName      string `json:"full_name"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
	}
	json.Unmarshal(body, &r)

	return &core.GitRepo{
		FullName: r.FullName, CloneURL: r.CloneURL, SSHURL: r.SSHURL,
		Description: r.Description, DefaultBranch: r.DefaultBranch, Private: r.Private,
	}, nil
}

func (g *GitHub) CreateWebhook(ctx context.Context, repoFullName, url, secret string, events []string) (string, error) {
	payload := map[string]any{
		"config": map[string]any{
			"url":          url,
			"content_type": "json",
			"secret":       secret,
		},
		"events": events,
		"active": true,
	}

	body, err := g.post(ctx, fmt.Sprintf("/repos/%s/hooks", repoFullName), payload)
	if err != nil {
		return "", err
	}

	var resp struct{ ID int `json:"id"` }
	json.Unmarshal(body, &resp)
	return fmt.Sprintf("%d", resp.ID), nil
}

func (g *GitHub) DeleteWebhook(ctx context.Context, repoFullName, webhookID string) error {
	_, err := g.do(ctx, http.MethodDelete, fmt.Sprintf("/repos/%s/hooks/%s", repoFullName, webhookID), nil)
	return err
}

func (g *GitHub) get(ctx context.Context, path string) ([]byte, error) {
	return g.do(ctx, http.MethodGet, path, nil)
}

func (g *GitHub) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return g.do(ctx, http.MethodPost, path, payload)
}

func (g *GitHub) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github API %s: HTTP %d", path, resp.StatusCode)
	}
	return respBody, nil
}
