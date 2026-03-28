package models

import "time"

// Application represents a deployed application.
type Application struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	TenantID   string    `json:"tenant_id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	SourceType string    `json:"source_type"`
	SourceURL  string    `json:"source_url"`
	Branch     string    `json:"branch"`
	Dockerfile string    `json:"dockerfile,omitempty"`
	BuildPack  string    `json:"build_pack,omitempty"`
	EnvVarsEnc string    `json:"-"`
	LabelsJSON string    `json:"labels_json,omitempty"`
	Replicas   int       `json:"replicas"`
	Port       int       `json:"port"`
	Status     string    `json:"status"`
	ServerID   string    `json:"server_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Project represents a logical grouping of applications.
type Project struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Environment string    `json:"environment"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Deployment represents a single deployment of an application.
type Deployment struct {
	ID            string     `json:"id"`
	AppID         string     `json:"app_id"`
	Version       int        `json:"version"`
	Image         string     `json:"image"`
	ContainerID   string     `json:"container_id"`
	Status        string     `json:"status"`
	BuildLog      string     `json:"build_log,omitempty"`
	CommitSHA     string     `json:"commit_sha,omitempty"`
	CommitMessage string     `json:"commit_message,omitempty"`
	TriggeredBy   string     `json:"triggered_by"`
	Strategy      string     `json:"strategy"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
