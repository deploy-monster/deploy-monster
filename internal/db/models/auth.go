package models

import "time"

// Role represents a permission role.
type Role struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id,omitempty"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	PermissionsJSON string    `json:"permissions_json"`
	IsBuiltin       bool      `json:"is_builtin"`
	CreatedAt       time.Time `json:"created_at"`
}

// TeamMember links a user to a tenant with a role.
type TeamMember struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	UserID       string     `json:"user_id"`
	RoleID       string     `json:"role_id"`
	InvitedBy    string     `json:"invited_by,omitempty"`
	Status       string     `json:"status"`
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// APIKey represents an API key for programmatic access.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TenantID   string     `json:"tenant_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"`
	ScopesJSON string     `json:"scopes_json"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Invitation represents a pending team invitation.
type Invitation struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Email      string     `json:"email"`
	RoleID     string     `json:"role_id"`
	InvitedBy  string     `json:"invited_by"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AuditLog represents an audit trail entry.
type AuditLog struct {
	ID           int64     `json:"id"`
	TenantID     string    `json:"tenant_id,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	DetailsJSON  string    `json:"details_json,omitempty"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
}
