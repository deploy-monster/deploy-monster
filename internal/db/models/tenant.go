package models

import "time"

// Tenant represents a team or organization.
type Tenant struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	AvatarURL    string    `json:"avatar_url"`
	PlanID       string    `json:"plan_id"`
	OwnerID      string    `json:"owner_id,omitempty"`
	ResellerID   string    `json:"reseller_id,omitempty"`
	Status       string    `json:"status"`
	LimitsJSON   string    `json:"limits_json,omitempty"`
	MetadataJSON string    `json:"metadata_json,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
