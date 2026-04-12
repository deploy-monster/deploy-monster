package models

import "time"

// VPSProvider represents a cloud VPS provider configuration.
type VPSProvider struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id,omitempty"`
	Type          string    `json:"type"`
	Name          string    `json:"name"`
	APITokenEnc   string    `json:"-"`
	DefaultRegion string    `json:"default_region"`
	DefaultSize   string    `json:"default_size"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// ManagedDB represents a managed database instance.
type ManagedDB struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Name           string    `json:"name"`
	Engine         string    `json:"engine"`
	Version        string    `json:"version"`
	Port           int       `json:"port"`
	CredentialsEnc string    `json:"-"`
	ContainerID    string    `json:"container_id"`
	VolumeID       string    `json:"volume_id"`
	ServerID       string    `json:"server_id,omitempty"`
	BackupSchedule string    `json:"backup_schedule"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// ComposeStack represents a Docker Compose stack definition.
type ComposeStack struct {
	ID         string    `json:"id"`
	AppID      string    `json:"app_id"`
	RawYAML    string    `json:"raw_yaml"`
	ParsedJSON string    `json:"parsed_json,omitempty"`
	Version    int       `json:"version"`
	SourceType string    `json:"source_type"`
	SourceURL  string    `json:"source_url"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// MarketplaceInstall tracks a marketplace template installation.
type MarketplaceInstall struct {
	ID           string    `json:"id"`
	TemplateSlug string    `json:"template_slug"`
	TenantID     string    `json:"tenant_id"`
	AppID        string    `json:"app_id,omitempty"`
	ConfigJSON   string    `json:"config_json,omitempty"`
	Version      string    `json:"version"`
	Status       string    `json:"status"`
	InstalledAt  time.Time `json:"installed_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Secret represents an encrypted secret.
type Secret struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id,omitempty"`
	ProjectID      string    `json:"project_id,omitempty"`
	AppID          string    `json:"app_id,omitempty"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Description    string    `json:"description"`
	Scope          string    `json:"scope"`
	CurrentVersion int       `json:"current_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SecretVersion represents a versioned secret value.
type SecretVersion struct {
	ID        string    `json:"id"`
	SecretID  string    `json:"secret_id"`
	Version   int       `json:"version"`
	ValueEnc  string    `json:"-"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
