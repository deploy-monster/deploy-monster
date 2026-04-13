package models

import "time"

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
