// Package topology handles the conversion of visual topology diagrams
// into deployable Docker Compose configurations.
package topology

import (
	"time"
)

// Topology represents a complete deployment topology
type Topology struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProjectID   string    `json:"projectId"`
	Environment string    `json:"environment"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	// Components
	Apps      []App      `json:"apps"`
	Databases []Database `json:"databases"`
	Domains   []Domain   `json:"domains"`
	Volumes   []Volume   `json:"volumes"`
	Workers   []Worker   `json:"workers"`

	// Connections
	Connections []Connection `json:"connections"`
}

// App represents a containerized application from Git
type App struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      ComponentStatus `json:"status"`
	Description string          `json:"description,omitempty"`

	// Source
	GitURL     string `json:"gitUrl"`
	Branch     string `json:"branch"`
	BuildPack  string `json:"buildPack"` // auto, nodejs, nextjs, go, python, rust, dockerfile
	Dockerfile string `json:"dockerfile,omitempty"`

	// Runtime
	Port     int `json:"port"`
	Replicas int `json:"replicas"`

	// Resources
	MemoryMB int `json:"memoryMB,omitempty"`
	CPU      int `json:"cpu,omitempty"` // millicores

	// Configuration
	EnvVars      map[string]string `json:"envVars,omitempty"`
	SecretRefs   map[string]string `json:"secretRefs,omitempty"` // env var -> secret path
	VolumeMounts []VolumeMount     `json:"volumeMounts,omitempty"`

	// Health
	HealthCheckPath string `json:"healthCheckPath,omitempty"`
	HealthCheckPort int    `json:"healthCheckPort,omitempty"`

	// Networking
	InternalOnly bool `json:"internalOnly,omitempty"`
}

// Database represents a managed or containerized database
type Database struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      ComponentStatus `json:"status"`
	Description string          `json:"description,omitempty"`

	// Engine
	Engine  DatabaseEngine `json:"engine"`
	Version string         `json:"version"`

	// Sizing
	SizeGB int `json:"sizeGB"`

	// Mode
	Managed  bool   `json:"managed,omitempty"`  // External managed DB
	External bool   `json:"external,omitempty"` // External connection
	ConnURL  string `json:"connUrl,omitempty"`  // For external DBs (alias: ConnURL)

	// Credentials (auto-generated if not provided)
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`

	// Configuration
	ExtraConfig map[string]string `json:"extraConfig,omitempty"`
}

// ConnectionData holds connection metadata
type ConnectionData struct {
	ID string `json:"id"`
}

// DatabaseEngine represents supported database types
type DatabaseEngine string

const (
	EnginePostgres DatabaseEngine = "postgres"
	EngineMySQL    DatabaseEngine = "mysql"
	EngineMariaDB  DatabaseEngine = "mariadb"
	EngineMongoDB  DatabaseEngine = "mongodb"
	EngineRedis    DatabaseEngine = "redis"
)

// Domain represents a custom domain with routing
type Domain struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     ComponentStatus `json:"status"`
	FQDN       string          `json:"fqdn"`
	SSLEnabled bool            `json:"sslEnabled"`
	SSLMODE    SSLMode         `json:"sslMode,omitempty"`

	// Routing
	TargetAppID string `json:"targetAppId,omitempty"`
	PathPrefix  string `json:"pathPrefix,omitempty"` // e.g., /api

	// DNS
	DNSProvider string `json:"dnsProvider,omitempty"`
	DNSZone     string `json:"dnsZone,omitempty"`
}

// SSLMode represents SSL certificate provisioning mode
type SSLMode string

const (
	SSLAuto   SSLMode = "auto"   // Let's Encrypt auto
	SSLManual SSLMode = "manual" // Manual certificate
	SSLNone   SSLMode = "none"   // No SSL
	SSLStrict SSLMode = "strict" // Force HTTPS
)

// Volume represents persistent storage
type Volume struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     ComponentStatus `json:"status"`
	SizeGB     int             `json:"sizeGB"`
	VolumeType VolumeType      `json:"volumeType"`
	MountPath  string          `json:"mountPath"` // Default mount path

	// For temporary volumes
	Temporary bool `json:"temporary,omitempty"`
}

// VolumeType represents storage backend
type VolumeType string

const (
	VolumeLocal VolumeType = "local"
	VolumeNFS   VolumeType = "nfs"
	VolumeTmpfs VolumeType = "tmpfs"
)

// VolumeMount represents a volume mounted to a container
type VolumeMount struct {
	VolumeID  string `json:"volumeId"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// Worker represents a background worker process
type Worker struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      ComponentStatus `json:"status"`
	Description string          `json:"description,omitempty"`

	// Source
	GitURL     string `json:"gitUrl"`
	Branch     string `json:"branch"`
	BuildPack  string `json:"buildPack"`
	Dockerfile string `json:"dockerfile,omitempty"`

	// Runtime
	Command  string `json:"command,omitempty"`
	Replicas int    `json:"replicas"`
	Schedule string `json:"schedule,omitempty"` // Cron schedule for scheduled workers

	// Resources
	MemoryMB int `json:"memoryMB,omitempty"`
	CPU      int `json:"cpu,omitempty"`

	// Configuration
	EnvVars      map[string]string `json:"envVars,omitempty"`
	SecretRefs   map[string]string `json:"secretRefs,omitempty"`
	VolumeMounts []VolumeMount     `json:"volumeMounts,omitempty"`
}

// Connection represents a relationship between components
type Connection struct {
	ID       string     `json:"id"`
	Type     ConnType   `json:"type"`
	SourceID string     `json:"sourceId"`
	TargetID string     `json:"targetId"`
	Label    string     `json:"label,omitempty"`
	Config   ConnConfig `json:"config,omitempty"`
}

// ConnType represents connection types
type ConnType string

const (
	ConnDependency ConnType = "dependency" // App depends on DB
	ConnMount      ConnType = "mount"      // App mounts Volume
	ConnRoute      ConnType = "route"      // Domain routes to App
	ConnNetwork    ConnType = "network"    // Apps in same network
)

// ConnConfig holds connection-specific configuration
type ConnConfig struct {
	// For mount connections
	MountPath string `json:"mountPath,omitempty"`
	ReadOnly  bool   `json:"readOnly,omitempty"`

	// For route connections
	PathPrefix string `json:"pathPrefix,omitempty"`
	Port       int    `json:"port,omitempty"`

	// For dependency connections
	EnvVarName string `json:"envVarName,omitempty"` // e.g., DATABASE_URL
}

// ComponentStatus represents the state of a component
type ComponentStatus string

const (
	StatusPending     ComponentStatus = "pending"
	StatusConfiguring ComponentStatus = "configuring"
	StatusBuilding    ComponentStatus = "building"
	StatusStarting    ComponentStatus = "starting"
	StatusRunning     ComponentStatus = "running"
	StatusStopped     ComponentStatus = "stopped"
	StatusError       ComponentStatus = "error"
)

// DeployRequest represents a topology deployment request
type DeployRequest struct {
	TopologyID string `json:"topologyId"`
	ProjectID  string `json:"projectId"`
	Env        string `json:"env"`

	// Options
	DryRun     bool `json:"dryRun,omitempty"`     // Generate but don't deploy
	ForceBuild bool `json:"forceBuild,omitempty"` // Force rebuild images
	NoCache    bool `json:"noCache,omitempty"`    // Build without cache
}

// DeployResult represents the result of a deployment
type DeployResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Duration string `json:"duration"`

	// Generated files
	ComposeYAML string `json:"composeYaml,omitempty"`
	Caddyfile   string `json:"caddyfile,omitempty"`
	EnvFile     string `json:"envFile,omitempty"`

	// Deployed resources
	Containers []string `json:"containers,omitempty"`
	Networks   []string `json:"networks,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`

	// Errors
	Errors []DeployError `json:"errors,omitempty"`
}

// DeployError represents an error during deployment
type DeployError struct {
	ComponentID   string `json:"componentId,omitempty"`
	ComponentName string `json:"componentName,omitempty"`
	Stage         string `json:"stage"` // validate, build, deploy
	Message       string `json:"message"`
}
