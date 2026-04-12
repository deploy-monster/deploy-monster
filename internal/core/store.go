package core

import (
	"context"
	"time"
)

// =====================================================
// STORE INTERFACE
// All data access goes through this interface.
// Implementations: SQLite (default), PostgreSQL (enterprise)
// Modules MUST use Store, never concrete DB types.
// =====================================================

// Store is the unified data access interface.
// Every module that needs data access receives this interface,
// not a concrete database implementation.
type Store interface {
	TenantStore
	UserStore
	AppStore
	DeploymentStore
	DomainStore
	ProjectStore
	RoleStore
	AuditStore
	SecretStore
	InviteStore
	UsageRecordStore
	BackupStore
	Close() error
	Ping(ctx context.Context) error
}

// TenantStore manages tenant CRUD.
type TenantStore interface {
	CreateTenant(ctx context.Context, tenant *Tenant) error
	GetTenant(ctx context.Context, id string) (*Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error)
	UpdateTenant(ctx context.Context, tenant *Tenant) error
	DeleteTenant(ctx context.Context, id string) error
}

// UserStore manages user CRUD.
type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	UpdateLastLogin(ctx context.Context, userID string) error
	CountUsers(ctx context.Context) (int, error)
	CreateUserWithMembership(ctx context.Context, email, passwordHash, name, status, tenantID, roleID string) (string, error)
}

// AppStore manages application CRUD.
type AppStore interface {
	CreateApp(ctx context.Context, app *Application) error
	GetApp(ctx context.Context, id string) (*Application, error)
	GetAppByName(ctx context.Context, tenantID, name string) (*Application, error)
	UpdateApp(ctx context.Context, app *Application) error
	ListAppsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]Application, int, error)
	ListAppsByProject(ctx context.Context, projectID string) ([]Application, error)
	UpdateAppStatus(ctx context.Context, id, status string) error
	DeleteApp(ctx context.Context, id string) error
}

// DeploymentStore manages deployment records.
type DeploymentStore interface {
	CreateDeployment(ctx context.Context, dep *Deployment) error
	// UpdateDeployment persists a mutation to an existing deployment row.
	// Added in Tier 100 alongside the Phase 3.1.2 restart storm test: pre-100
	// the deploy pipeline mutated deployment.Status only in memory, so every
	// row in the deployments table was permanently stuck in "deploying".
	UpdateDeployment(ctx context.Context, dep *Deployment) error
	GetLatestDeployment(ctx context.Context, appID string) (*Deployment, error)
	ListDeploymentsByApp(ctx context.Context, appID string, limit int) ([]Deployment, error)
	// ListDeploymentsByStatus returns every deployment in the given status.
	// Used by deploy.Module.Start to reclaim in-flight deployments left
	// behind by a crashed master (Phase 3.1.2 restart storm).
	ListDeploymentsByStatus(ctx context.Context, status string) ([]Deployment, error)
	GetNextDeployVersion(ctx context.Context, appID string) (int, error)
}

// DomainStore manages domain CRUD.
type DomainStore interface {
	CreateDomain(ctx context.Context, domain *Domain) error
	GetDomainByFQDN(ctx context.Context, fqdn string) (*Domain, error)
	ListDomainsByApp(ctx context.Context, appID string) ([]Domain, error)
	DeleteDomain(ctx context.Context, id string) error
	DeleteDomainsByApp(ctx context.Context, appID string) (int, error)
	ListAllDomains(ctx context.Context) ([]Domain, error)
}

// ProjectStore manages project CRUD.
type ProjectStore interface {
	CreateProject(ctx context.Context, project *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjectsByTenant(ctx context.Context, tenantID string) ([]Project, error)
	DeleteProject(ctx context.Context, id string) error
	CreateTenantWithDefaults(ctx context.Context, name, slug string) (string, error)
}

// RoleStore manages roles and team membership.
type RoleStore interface {
	GetRole(ctx context.Context, roleID string) (*Role, error)
	GetUserMembership(ctx context.Context, userID string) (*TeamMember, error)
	ListRoles(ctx context.Context, tenantID string) ([]Role, error)
}

// AuditStore manages audit log entries.
type AuditStore interface {
	CreateAuditLog(ctx context.Context, entry *AuditEntry) error
	ListAuditLogs(ctx context.Context, tenantID string, limit, offset int) ([]AuditEntry, int, error)
}

// SecretStore manages encrypted secret metadata and versions.
type SecretStore interface {
	CreateSecret(ctx context.Context, secret *Secret) error
	CreateSecretVersion(ctx context.Context, version *SecretVersion) error
	ListSecretsByTenant(ctx context.Context, tenantID string) ([]Secret, error)
	GetSecretByScopeAndName(ctx context.Context, scope, name string) (*Secret, error)
	GetLatestSecretVersion(ctx context.Context, secretID string) (*SecretVersion, error)
	ListAllSecretVersions(ctx context.Context) ([]SecretVersion, error)
	UpdateSecretVersionValue(ctx context.Context, id, valueEnc string) error
}

// InviteStore manages team invitations.
type InviteStore interface {
	CreateInvite(ctx context.Context, invite *Invitation) error
	ListInvitesByTenant(ctx context.Context, tenantID string) ([]Invitation, error)
	ListAllTenants(ctx context.Context, limit, offset int) ([]Tenant, int, error)
}

// =====================================================
// STORE DATA MODELS
// DB-agnostic data models used by Store interface.
// These replace the db/models package for cross-module use.
// =====================================================

// Tenant represents a team or organization.
type Tenant struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	AvatarURL    string    `json:"avatar_url"`
	PlanID       string    `json:"plan_id"`
	OwnerID      string    `json:"owner_id,omitempty"`
	Status       string    `json:"status"`
	LimitsJSON   string    `json:"limits_json,omitempty"`
	MetadataJSON string    `json:"metadata_json,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// User represents a platform user.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Name         string     `json:"name"`
	AvatarURL    string     `json:"avatar_url"`
	Status       string     `json:"status"`
	TOTPEnabled  bool       `json:"totp_enabled"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

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

// Domain represents a domain mapped to an application.
type Domain struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	FQDN        string    `json:"fqdn"`
	Type        string    `json:"type"`
	DNSProvider string    `json:"dns_provider"`
	DNSSynced   bool      `json:"dns_synced"`
	Verified    bool      `json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
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
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	RoleID    string    `json:"role_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEntry represents an audit log record.
type AuditEntry struct {
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

// Secret represents an encrypted secret (metadata only, no values).
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

// SecretVersion represents a versioned encrypted secret value.
type SecretVersion struct {
	ID        string    `json:"id"`
	SecretID  string    `json:"secret_id"`
	Version   int       `json:"version"`
	ValueEnc  string    `json:"-"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Invitation represents a team invitation.
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

// UsageRecord tracks resource usage for billing.
type UsageRecord struct {
	ID         int64     `json:"id"`
	TenantID   string    `json:"tenant_id"`
	AppID      string    `json:"app_id,omitempty"`
	MetricType string    `json:"metric_type"`
	Value      float64   `json:"value"`
	HourBucket time.Time `json:"hour_bucket"`
	CreatedAt  time.Time `json:"created_at"`
}

// UsageRecordStore persists resource usage metrics.
type UsageRecordStore interface {
	CreateUsageRecord(ctx context.Context, record *UsageRecord) error
	ListUsageRecordsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]UsageRecord, int, error)
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

// BackupStore persists backup metadata.
type BackupStore interface {
	CreateBackup(ctx context.Context, backup *Backup) error
	ListBackupsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]Backup, int, error)
	UpdateBackupStatus(ctx context.Context, id, status string, sizeBytes int64) error
}
