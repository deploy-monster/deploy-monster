package models

import "time"

// Server represents a physical or virtual server in the cluster.
type Server struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id,omitempty"`
	Hostname         string    `json:"hostname"`
	IPAddress        string    `json:"ip_address"`
	Role             string    `json:"role"`
	ProviderType     string    `json:"provider_type"`
	ProviderRef      string    `json:"provider_ref,omitempty"`
	SSHPort          int       `json:"ssh_port"`
	SSHKeyID         string    `json:"ssh_key_id,omitempty"`
	DockerVersion    string    `json:"docker_version"`
	CPUCores         int       `json:"cpu_cores"`
	RAMMB            int       `json:"ram_mb"`
	DiskMB           int       `json:"disk_mb"`
	MonthlyCostCents int       `json:"monthly_cost_cents"`
	SwarmJoined      bool      `json:"swarm_joined"`
	AgentStatus      string    `json:"agent_status"`
	LabelsJSON       string    `json:"labels_json,omitempty"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

// Volume represents a Docker volume attached to an application.
type Volume struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id,omitempty"`
	Name      string    `json:"name"`
	MountPath string    `json:"mount_path"`
	SizeMB    int       `json:"size_mb"`
	Driver    string    `json:"driver"`
	ServerID  string    `json:"server_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Backup represents a backup record.
type Backup struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	StorageTarget string     `json:"storage_target"`
	FilePath      string     `json:"file_path"`
	SizeBytes     int64      `json:"size_bytes"`
	Encryption    string     `json:"encryption"`
	Status        string     `json:"status"`
	Scheduled     bool       `json:"scheduled"`
	RetentionDays int        `json:"retention_days"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
