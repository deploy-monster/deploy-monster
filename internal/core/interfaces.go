package core

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// =====================================================
// SERVICE INTERFACES
// Modules communicate via these interfaces, not via
// direct package imports. This enables true modularity —
// any module can be developed, tested, and replaced
// independently.
// =====================================================

// --- Container Runtime ---

// ContainerRuntime is the interface for container operations.
// Implemented by: deploy module
type ContainerRuntime interface {
	Ping() error
	CreateAndStart(ctx context.Context, opts ContainerOpts) (string, error)
	Stop(ctx context.Context, containerID string, timeoutSec int) error
	Remove(ctx context.Context, containerID string, force bool) error
	Restart(ctx context.Context, containerID string) error
	Logs(ctx context.Context, containerID string, tail string, follow bool) (io.ReadCloser, error)
	ListByLabels(ctx context.Context, labels map[string]string) ([]ContainerInfo, error)
	Exec(ctx context.Context, containerID string, cmd []string) (string, error)
	Stats(ctx context.Context, containerID string) (*ContainerStats, error)
	ImagePull(ctx context.Context, image string) error
	ImageList(ctx context.Context) ([]ImageInfo, error)
	ImageRemove(ctx context.Context, imageID string) error
	NetworkList(ctx context.Context) ([]NetworkInfo, error)
	VolumeList(ctx context.Context) ([]VolumeInfo, error)
}

// ContainerOpts holds options for creating a container.
type ContainerOpts struct {
	Name              string
	Image             string
	Env               []string
	Labels            map[string]string
	Ports             map[string]string // "containerPort": "hostPort"
	Volumes           map[string]string // "hostPath": "containerPath"
	Network           string
	CPUQuota          int64
	MemoryMB          int64
	RestartPolicy     string
	AllowDockerSocket bool // Explicitly allow Docker socket mount (marketplace apps only)
	Privileged        bool // Run in privileged mode (marketplace apps only)
}

// dangerousPaths contains host paths that tenant containers must never mount
// unless explicitly allowed. The Docker socket grants root-level host control.
var dangerousPaths = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/var/run/docker",
}

// ValidateVolumePaths checks volume mount host paths for path traversal attacks
// and blocks access to sensitive host paths (Docker socket).
func (o *ContainerOpts) ValidateVolumePaths() error {
	for hostPath := range o.Volumes {
		if strings.Contains(hostPath, "\x00") {
			return fmt.Errorf("volume host path contains null byte")
		}

		// SECURITY FIX: Pre-cleaning check for raw traversal attempts
		// This catches .. before Clean() resolves them
		if strings.Contains(hostPath, "..") {
			return fmt.Errorf("volume host path %q contains path traversal", hostPath)
		}

		cleaned := filepath.Clean(hostPath)

		// SECURITY FIX: Post-cleaning check - ensure path is still absolute and not escaped
		// filepath.Clean() resolves .. but we need to verify the result
		if strings.Contains(cleaned, "..") {
			return fmt.Errorf("volume host path %q contains path traversal after cleaning", hostPath)
		}

		// SECURITY FIX: Verify the path is absolute after cleaning
		// This prevents relative paths that might resolve outside expected directories
		if !filepath.IsAbs(cleaned) {
			return fmt.Errorf("volume host path %q must be absolute", hostPath)
		}

		// SECURITY FIX: Check that cleaned path doesn't resolve to root or system directories
		// Use forward-slash normalized path for cross-platform comparison
		normalizedPath := strings.ReplaceAll(cleaned, "\\", "/")

		// Block traversal to root
		if normalizedPath == "/" || normalizedPath == "\\" {
			return fmt.Errorf("volume host path %q cannot be root directory", hostPath)
		}

		// Block Docker socket mounts unless explicitly allowed
		if !o.AllowDockerSocket {
			for _, dangerous := range dangerousPaths {
				if normalizedPath == dangerous {
					return fmt.Errorf("volume host path %q is blocked — Docker socket access requires AllowDockerSocket", hostPath)
				}
			}
		}
	}
	return nil
}

// ApplyResourceDefaults sets CPU and memory limits from config defaults
// when the caller hasn't specified them. This prevents unbounded containers.
func (o *ContainerOpts) ApplyResourceDefaults(defaultCPU, defaultMemoryMB int64) {
	if o.CPUQuota <= 0 && defaultCPU > 0 {
		o.CPUQuota = defaultCPU
	}
	if o.MemoryMB <= 0 && defaultMemoryMB > 0 {
		o.MemoryMB = defaultMemoryMB
	}
}

// ContainerInfo holds basic container information.
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	Status  string
	State   string
	Labels  map[string]string
	Created int64
}

// ContainerStats holds real-time resource usage statistics for a container.
type ContainerStats struct {
	CPUPercent    float64
	MemoryUsage   int64
	MemoryLimit   int64
	MemoryPercent float64
	NetworkRx     int64
	NetworkTx     int64
	BlockRead     int64
	BlockWrite    int64
	PIDs          int
	// Health status from Docker healthcheck: "healthy", "unhealthy", "starting", or "" if no healthcheck
	Health string
	// Running indicates if the container is currently running
	Running bool
}

// ImageInfo holds basic Docker image information.
type ImageInfo struct {
	ID      string
	Tags    []string
	Size    int64
	Created int64
}

// NetworkInfo holds basic Docker network information.
type NetworkInfo struct {
	ID     string
	Name   string
	Driver string
	Scope  string
}

// VolumeInfo holds basic Docker volume information.
type VolumeInfo struct {
	Name       string
	Driver     string
	Mountpoint string
	CreatedAt  string
}

// --- SSH ---

// SSHClient is the interface for SSH connection management.
// Implemented by: vps module
type SSHClient interface {
	Execute(ctx context.Context, serverID, command string) (string, error)
	Upload(ctx context.Context, serverID, localPath, remotePath string) error
}

// --- Secrets ---

// SecretResolver is the interface for resolving secret references.
// Implemented by: secrets module
// Usage: ${SECRET:name} syntax in env vars, compose files, etc.
type SecretResolver interface {
	Resolve(scope, name string) (string, error)
	ResolveAll(scope string, template string) (string, error)
}

// --- Notification ---

// NotificationSender sends notifications through various channels.
// Implemented by: notifications module
type NotificationSender interface {
	Send(ctx context.Context, notification Notification) error
}

// Notification represents a message to be sent via a notification channel.
type Notification struct {
	Channel   string // email, slack, discord, telegram, webhook
	Recipient string // Email address, webhook URL, channel ID, etc.
	Subject   string
	Body      string
	Format    string            // text, html, markdown
	Metadata  map[string]string // Channel-specific metadata
}

// --- DNS ---

// DNSProvider manages DNS records for a specific provider.
// Implemented by: dns provider modules (cloudflare, route53, etc.)
type DNSProvider interface {
	Name() string
	CreateRecord(ctx context.Context, record DNSRecord) error
	UpdateRecord(ctx context.Context, record DNSRecord) error
	DeleteRecord(ctx context.Context, record DNSRecord) error
	Verify(ctx context.Context, fqdn string) (bool, error)
}

// DNSRecord represents a DNS record.
type DNSRecord struct {
	ID      string
	Type    string // A, AAAA, CNAME, TXT
	Name    string // Subdomain or FQDN
	Value   string // IP address, target, etc.
	TTL     int
	Proxied bool // Cloudflare proxy toggle
}

// --- Backup Storage ---

// BackupStorage is the interface for backup storage targets.
// Implemented by: backup storage modules (local, s3, sftp, etc.)
type BackupStorage interface {
	Name() string
	Upload(ctx context.Context, key string, reader io.Reader, size int64) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]BackupEntry, error)
}

// BackupEntry represents a stored backup.
type BackupEntry struct {
	Key       string
	Size      int64
	CreatedAt int64
}

// --- VPS Provider ---

// VPSProvisioner provisions and manages virtual servers.
// Implemented by: vps provider modules (hetzner, digitalocean, vultr, etc.)
type VPSProvisioner interface {
	Name() string
	ListRegions(ctx context.Context) ([]VPSRegion, error)
	ListSizes(ctx context.Context, region string) ([]VPSSize, error)
	Create(ctx context.Context, opts VPSCreateOpts) (*VPSInstance, error)
	Delete(ctx context.Context, instanceID string) error
	Status(ctx context.Context, instanceID string) (string, error)
}

// VPSRegion represents a provider region.
type VPSRegion struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VPSSize represents a server size/plan.
type VPSSize struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	CPUs      int     `json:"cpus"`
	MemoryMB  int     `json:"memory_mb"`
	DiskGB    int     `json:"disk_gb"`
	PriceHour float64 `json:"price_hour"`
}

// VPSCreateOpts holds options for creating a VPS instance.
type VPSCreateOpts struct {
	Name     string
	Region   string
	Size     string
	Image    string
	SSHKeyID string
	UserData string
}

// VPSInstance represents a provisioned VPS.
type VPSInstance struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Status    string `json:"status"`
	Region    string `json:"region"`
	Size      string `json:"size"`
}

// --- Git Provider ---

// GitProvider interfaces with a Git hosting provider's API.
// Implemented by: gitsources provider modules (github, gitlab, gitea, etc.)
type GitProvider interface {
	Name() string
	ListRepos(ctx context.Context, page, perPage int) ([]GitRepo, error)
	ListBranches(ctx context.Context, repoFullName string) ([]string, error)
	GetRepoInfo(ctx context.Context, repoFullName string) (*GitRepo, error)
	CreateWebhook(ctx context.Context, repoFullName, url, secret string, events []string) (string, error)
	DeleteWebhook(ctx context.Context, repoFullName, webhookID string) error
}

// GitRepo represents a Git repository.
type GitRepo struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// =====================================================
// DATABASE ACCESS
// =====================================================

// Database wraps the SQLite and BBolt stores as a unified data access layer.
type Database struct {
	SQL         *sql.DB
	Bolt        BoltStorer
	Snapshotter DBSnapshotter // optional, set when the DB supports snapshot backup
}

// DBSnapshotter creates consistent point-in-time database copies.
// Implemented by SQLiteDB via WAL checkpoint + VACUUM INTO.
type DBSnapshotter interface {
	SnapshotBackup(ctx context.Context, destPath string) error
}

// BoltBatchItem represents a single write in a batch operation.
type BoltBatchItem struct {
	Bucket string
	Key    string
	Value  any
	TTL    int64 // seconds, 0 = no expiry
}

// BoltStorer is the interface for BBolt key-value operations.
type BoltStorer interface {
	Set(bucket, key string, value any, ttlSeconds int64) error
	BatchSet(items []BoltBatchItem) error // write multiple keys in one transaction
	Get(bucket, key string, dest any) error
	Delete(bucket, key string) error
	List(bucket string) ([]string, error)
	Close() error
	// GetAPIKeyByPrefix retrieves an API key by its key prefix (first 8 chars).
	// Used for API key validation in middleware.
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error)
	// GetWebhookSecret retrieves the webhook secret for signature verification.
	// Returns the secret hash stored for the given webhook ID.
	GetWebhookSecret(webhookID string) (string, error)
}

// =====================================================
// SERVICE REGISTRY
// Modules register their service implementations here
// so other modules can look them up by interface type
// without importing the concrete package.
// =====================================================

// Services holds references to shared service implementations.
// Populated by modules during Init() phase.
// Other modules read from here instead of doing type assertions
// on Registry.Get() results.
type Services struct {
	Container     ContainerRuntime
	SSH           SSHClient
	Secrets       SecretResolver
	Notifications NotificationSender

	// Provider registries — support multiple implementations
	dnsProviders    map[string]DNSProvider
	backupStorages  map[string]BackupStorage
	vpsProvisioners map[string]VPSProvisioner
	gitProviders    map[string]GitProvider
}

// NewServices creates an empty services registry.
func NewServices() *Services {
	return &Services{
		dnsProviders:    make(map[string]DNSProvider),
		backupStorages:  make(map[string]BackupStorage),
		vpsProvisioners: make(map[string]VPSProvisioner),
		gitProviders:    make(map[string]GitProvider),
	}
}

// RegisterDNSProvider adds a DNS provider.
func (s *Services) RegisterDNSProvider(name string, provider DNSProvider) {
	s.dnsProviders[name] = provider
}

// DNSProvider returns a DNS provider by name.
func (s *Services) DNSProvider(name string) DNSProvider {
	return s.dnsProviders[name]
}

// DNSProviders returns all registered DNS provider names.
func (s *Services) DNSProviders() []string {
	names := make([]string, 0, len(s.dnsProviders))
	for name := range s.dnsProviders {
		names = append(names, name)
	}
	return names
}

// RegisterBackupStorage adds a backup storage target.
func (s *Services) RegisterBackupStorage(name string, storage BackupStorage) {
	s.backupStorages[name] = storage
}

// BackupStorage returns a backup storage by name.
func (s *Services) BackupStorage(name string) BackupStorage {
	return s.backupStorages[name]
}

// RegisterVPSProvisioner adds a VPS provisioner.
func (s *Services) RegisterVPSProvisioner(name string, provisioner VPSProvisioner) {
	s.vpsProvisioners[name] = provisioner
}

// VPSProvisioner returns a VPS provisioner by name.
func (s *Services) VPSProvisioner(name string) VPSProvisioner {
	return s.vpsProvisioners[name]
}

// RegisterGitProvider adds a Git provider.
func (s *Services) RegisterGitProvider(name string, provider GitProvider) {
	s.gitProviders[name] = provider
}

// GitProvider returns a Git provider by name.
func (s *Services) GitProvider(name string) GitProvider {
	return s.gitProviders[name]
}

// GitProviders returns all registered Git provider names.
func (s *Services) GitProviders() []string {
	names := make([]string, 0, len(s.gitProviders))
	for name := range s.gitProviders {
		names = append(names, name)
	}
	return names
}
