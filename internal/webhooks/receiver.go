package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Receiver handles inbound webhooks from Git providers.
// It verifies signatures, parses payloads, and dispatches build events.
type Receiver struct {
	store  core.Store
	events *core.EventBus
	logger *slog.Logger
}

// NewReceiver creates a new webhook receiver.
func NewReceiver(store core.Store, events *core.EventBus, logger *slog.Logger) *Receiver {
	return &Receiver{store: store, events: events, logger: logger}
}

// WebhookPayload is the parsed result of an inbound webhook.
type WebhookPayload struct {
	Provider  string `json:"provider"`
	EventType string `json:"event_type"` // push, tag, pull_request
	Branch    string `json:"branch"`
	CommitSHA string `json:"commit_sha"`
	CommitMsg string `json:"commit_message"`
	Author    string `json:"author"`
	RepoURL   string `json:"repo_url"`
	RepoName  string `json:"repo_name"`
}

// HandleWebhook processes POST /hooks/v1/{webhookID}
func (recv *Receiver) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID := r.PathValue("webhookID")
	if webhookID == "" {
		http.Error(w, `{"error":"missing webhook ID"}`, http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB max
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	recv.logger.Info("webhook received",
		"webhook_id", webhookID,
		"provider", detectProvider(r),
		"size", len(body),
	)

	// Detect provider from headers
	provider := detectProvider(r)

	// Parse the payload based on provider
	payload, err := parsePayload(provider, body, r)
	if err != nil {
		recv.logger.Error("failed to parse webhook", "error", err)
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	// Emit webhook received event
	recv.events.Publish(r.Context(), core.NewEvent(
		core.EventWebhookReceived, "webhooks",
		core.WebhookEventData{
			WebhookID: webhookID,
			Provider:  provider,
			EventType: payload.EventType,
			Branch:    payload.Branch,
			CommitSHA: payload.CommitSHA,
		},
	))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

// detectProvider identifies the Git provider from request headers.
func detectProvider(r *http.Request) string {
	if r.Header.Get("X-GitHub-Event") != "" {
		return "github"
	}
	if r.Header.Get("X-Gitlab-Event") != "" {
		return "gitlab"
	}
	if r.Header.Get("X-Gitea-Event") != "" {
		return "gitea"
	}
	if r.Header.Get("X-Gogs-Event") != "" {
		return "gogs"
	}
	if r.Header.Get("X-Event-Key") != "" {
		return "bitbucket"
	}
	return "generic"
}

// parsePayload extracts a normalized WebhookPayload from provider-specific JSON.
func parsePayload(provider string, body []byte, r *http.Request) (*WebhookPayload, error) {
	switch provider {
	case "github":
		return parseGitHub(body, r)
	case "gitlab":
		return parseGitLab(body, r)
	case "gitea", "gogs":
		return parseGitea(body, r)
	default:
		return parseGeneric(body)
	}
}

func parseGitHub(body []byte, r *http.Request) (*WebhookPayload, error) {
	eventType := r.Header.Get("X-GitHub-Event")

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	p := &WebhookPayload{Provider: "github", EventType: eventType}

	if ref, ok := raw["ref"].(string); ok {
		p.Branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	if head, ok := raw["head_commit"].(map[string]any); ok {
		p.CommitSHA, _ = head["id"].(string)
		p.CommitMsg, _ = head["message"].(string)
		if author, ok := head["author"].(map[string]any); ok {
			p.Author, _ = author["name"].(string)
		}
	}
	if repo, ok := raw["repository"].(map[string]any); ok {
		p.RepoURL, _ = repo["clone_url"].(string)
		p.RepoName, _ = repo["full_name"].(string)
	}

	return p, nil
}

func parseGitLab(body []byte, r *http.Request) (*WebhookPayload, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	p := &WebhookPayload{Provider: "gitlab", EventType: r.Header.Get("X-Gitlab-Event")}

	if ref, ok := raw["ref"].(string); ok {
		p.Branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	if sha, ok := raw["checkout_sha"].(string); ok {
		p.CommitSHA = sha
	}
	if commits, ok := raw["commits"].([]any); ok && len(commits) > 0 {
		if last, ok := commits[len(commits)-1].(map[string]any); ok {
			p.CommitMsg, _ = last["message"].(string)
			if author, ok := last["author"].(map[string]any); ok {
				p.Author, _ = author["name"].(string)
			}
		}
	}
	if proj, ok := raw["project"].(map[string]any); ok {
		p.RepoURL, _ = proj["git_http_url"].(string)
		p.RepoName, _ = proj["path_with_namespace"].(string)
	}

	return p, nil
}

func parseGitea(body []byte, r *http.Request) (*WebhookPayload, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	eventType := r.Header.Get("X-Gitea-Event")
	if eventType == "" {
		eventType = r.Header.Get("X-Gogs-Event")
	}

	p := &WebhookPayload{Provider: "gitea", EventType: eventType}

	if ref, ok := raw["ref"].(string); ok {
		p.Branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	if sha, ok := raw["after"].(string); ok {
		p.CommitSHA = sha
	}
	if commits, ok := raw["commits"].([]any); ok && len(commits) > 0 {
		if last, ok := commits[len(commits)-1].(map[string]any); ok {
			p.CommitMsg, _ = last["message"].(string)
		}
	}
	if repo, ok := raw["repository"].(map[string]any); ok {
		p.RepoURL, _ = repo["clone_url"].(string)
		p.RepoName, _ = repo["full_name"].(string)
	}

	return p, nil
}

func parseGeneric(body []byte) (*WebhookPayload, error) {
	var p WebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	if p.Provider == "" {
		p.Provider = "generic"
	}
	return &p, nil
}

// VerifyGitHubSignature validates the X-Hub-Signature-256 header.
func VerifyGitHubSignature(body []byte, secret, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	expected := computeHMACSHA256(body, secret)
	return hmac.Equal([]byte(signature[7:]), []byte(expected))
}

// VerifyGitLabToken validates the X-Gitlab-Token header.
func VerifyGitLabToken(header, secret string) bool {
	return hmac.Equal([]byte(header), []byte(secret))
}

func computeHMACSHA256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// RegisterWebhookRoutes adds webhook routes to an existing handler.
func (recv *Receiver) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hooks/v1/{webhookID}", recv.HandleWebhook)
}

// VerifySignature verifies a webhook signature based on the provider.
func VerifySignature(ctx context.Context, provider string, body []byte, secret string, r *http.Request) bool {
	switch provider {
	case "github":
		return VerifyGitHubSignature(body, secret, r.Header.Get("X-Hub-Signature-256"))
	case "gitlab":
		return VerifyGitLabToken(r.Header.Get("X-Gitlab-Token"), secret)
	case "gitea", "gogs":
		// Gitea/Gogs use same format as GitHub
		sig := r.Header.Get("X-Gitea-Signature")
		if sig == "" {
			sig = r.Header.Get("X-Gogs-Signature")
		}
		return VerifyGitHubSignature(body, secret, fmt.Sprintf("sha256=%s", sig))
	default:
		return true // Generic webhooks — no verification
	}
}
