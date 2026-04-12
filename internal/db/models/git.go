package models

import "time"

// GitSource represents a connected Git provider.
type GitSource struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	BaseURL      string    `json:"base_url"`
	APIURL       string    `json:"api_url"`
	AuthType     string    `json:"auth_type"`
	TokenEnc     string    `json:"-"`
	OAuthDataEnc string    `json:"-"`
	SSHKeyID     string    `json:"ssh_key_id,omitempty"`
	Verified     bool      `json:"verified"`
	CreatedAt    time.Time `json:"created_at"`
}

// Webhook represents a Git webhook configuration.
type Webhook struct {
	ID              string     `json:"id"`
	AppID           string     `json:"app_id"`
	GitSourceID     string     `json:"git_source_id,omitempty"`
	SecretHash      string     `json:"-"`
	EventsJSON      string     `json:"events_json"`
	BranchFilter    string     `json:"branch_filter"`
	AutoDeploy      bool       `json:"auto_deploy"`
	Status          string     `json:"status"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// WebhookLog records a webhook delivery.
type WebhookLog struct {
	ID           string     `json:"id"`
	WebhookID    string     `json:"webhook_id"`
	EventType    string     `json:"event_type"`
	PayloadHash  string     `json:"payload_hash"`
	CommitSHA    string     `json:"commit_sha"`
	Branch       string     `json:"branch"`
	Status       string     `json:"status"`
	DeploymentID string     `json:"deployment_id,omitempty"`
	ReceivedAt   time.Time  `json:"received_at"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
}
