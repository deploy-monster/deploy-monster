package models

import "time"

// GitSource represents a Git provider connection (GitHub, GitLab, etc.).
// Used for OAuth-based webhook auto-registration.
type GitSource struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	UserID      string    `json:"user_id"`
	Provider    string    `json:"provider"` // github, gitlab, gitea, bitbucket
	AccessToken string    `json:"-"`        // encrypted OAuth token
	RefreshToken string   `json:"-"`        // encrypted refresh token
	TokenExpiry int64     `json:"token_expiry"`
	RepoScope   bool      `json:"repo_scope"` // true = full repo access, false = read-only
	AutoRegister bool     `json:"auto_register"` // auto-create webhooks on new apps
	WebhookURL  string    `json:"webhook_url"` // base URL for webhook callbacks
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}