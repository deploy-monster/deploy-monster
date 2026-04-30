package models

import "time"

// RegistryAuth stores credentials for private Docker registries.
type RegistryAuth struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`          // e.g., "docker-hub", "ghcr-io"
	RegistryURL  string    `json:"registry_url"`  // e.g., "https://index.docker.io/v1/"
	Username     string    `json:"username"`
	PasswordEnc  string    `json:"-"`             // encrypted password
	Email        string    `json:"email,omitempty"`
	AuthJSON     string    `json:"auth_json"`     // base64 encoded {username:password}
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Validate checks if the registry auth has required fields.
func (r *RegistryAuth) Validate() []string {
	var issues []string
	if r.RegistryURL == "" {
		issues = append(issues, "registry_url is required")
	}
	if r.Username == "" {
		issues = append(issues, "username is required")
	}
	if r.PasswordEnc == "" && r.AuthJSON == "" {
		issues = append(issues, "password or auth_json is required")
	}
	return issues
}