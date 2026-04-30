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

var _ core.GitProvider = (*Bitbucket)(nil)

// Bitbucket implements core.GitProvider for Bitbucket Cloud.
type Bitbucket struct {
	token  string
	client *http.Client
}

func NewBitbucket(token string) core.GitProvider {
	return &Bitbucket{
		token:  token,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *Bitbucket) Name() string { return "bitbucket" }

func (b *Bitbucket) ListRepos(ctx context.Context, page, perPage int) ([]core.GitRepo, error) {
	body, err := b.get(ctx, fmt.Sprintf("/repositories?role=member&page=%d&pagelen=%d", page, perPage))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Values []struct {
			FullName string `json:"full_name"`
			Links    struct {
				Clone []struct {
					Href string `json:"href"`
					Name string `json:"name"`
				} `json:"clone"`
			} `json:"links"`
			Description string `json:"description"`
			MainBranch  struct {
				Name string `json:"name"`
			} `json:"mainbranch"`
			IsPrivate bool `json:"is_private"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bitbucket repos parse: %w", err)
	}

	repos := make([]core.GitRepo, len(resp.Values))
	for i, r := range resp.Values {
		repos[i] = core.GitRepo{
			FullName:      r.FullName,
			Description:   r.Description,
			DefaultBranch: r.MainBranch.Name,
			Private:       r.IsPrivate,
		}
		for _, link := range r.Links.Clone {
			switch link.Name {
			case "https":
				repos[i].CloneURL = link.Href
			case "ssh":
				repos[i].SSHURL = link.Href
			}
		}
	}
	return repos, nil
}

func (b *Bitbucket) ListBranches(ctx context.Context, repoFullName string) ([]string, error) {
	body, err := b.get(ctx, fmt.Sprintf("/repositories/%s/refs/branches?pagelen=100", repoFullName))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Values []struct {
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bitbucket branches parse: %w", err)
	}

	branches := make([]string, len(resp.Values))
	for i, br := range resp.Values {
		branches[i] = br.Name
	}
	return branches, nil
}

func (b *Bitbucket) GetRepoInfo(ctx context.Context, repoFullName string) (*core.GitRepo, error) {
	body, err := b.get(ctx, "/repositories/"+repoFullName)
	if err != nil {
		return nil, err
	}
	var r struct {
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		MainBranch  struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
		IsPrivate bool `json:"is_private"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("bitbucket repo parse: %w", err)
	}

	return &core.GitRepo{
		FullName:      r.FullName,
		Description:   r.Description,
		DefaultBranch: r.MainBranch.Name,
		Private:       r.IsPrivate,
	}, nil
}

func (b *Bitbucket) CreateWebhook(ctx context.Context, repoFullName, url, secret string, events []string) (string, error) {
	bbEvents := make([]string, len(events))
	for i, e := range events {
		switch e {
		case "push":
			bbEvents[i] = "repo:push"
		case "pull_request":
			bbEvents[i] = "pullrequest:created"
		default:
			bbEvents[i] = e
		}
	}

	payload := map[string]any{
		"description": "DeployMonster webhook",
		"url":         url,
		"active":      true,
		"events":      bbEvents,
		"secret":      secret,
	}
	body, err := b.post(ctx, fmt.Sprintf("/repositories/%s/hooks", repoFullName), payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("bitbucket webhook parse: %w", err)
	}
	return resp.UUID, nil
}

func (b *Bitbucket) DeleteWebhook(ctx context.Context, repoFullName, webhookID string) error {
	_, err := b.do(ctx, http.MethodDelete, fmt.Sprintf("/repositories/%s/hooks/%s", repoFullName, webhookID), nil)
	return err
}

func (b *Bitbucket) get(ctx context.Context, path string) ([]byte, error) {
	return b.do(ctx, http.MethodGet, path, nil)
}

func (b *Bitbucket) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return b.do(ctx, http.MethodPost, path, payload)
}

func (b *Bitbucket) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.bitbucket.org/2.0"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bitbucket API %s: HTTP %d", path, resp.StatusCode)
	}
	return respBody, nil
}
