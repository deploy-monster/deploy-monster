# DeployMonster — IMPLEMENTATION.md

> **Companion to**: SPECIFICATION.md v1.0
> **Language**: Go 1.23+
> **Paradigm**: Modular monolith, event-driven, minimal dependencies
> **Build**: Single binary with embedded React UI via `embed.FS`

---

## 1. PROJECT BOOTSTRAP

### 1.1 Go Module Init

```bash
mkdir deploy-monster && cd deploy-monster
go mod init github.com/deploy-monster/deploy-monster
```

### 1.2 Directory Skeleton

```bash
mkdir -p cmd/deploymonster
mkdir -p internal/{core,auth,api,db,ingress,deploy,build,discovery,dns,resource,backup,database,swarm,vps,gitsources,compose,marketplace,mcp,notifications,webhooks,secrets,billing,enterprise}
mkdir -p internal/api/{handlers,middleware,ws}
mkdir -p internal/ingress/{middleware,lb}
mkdir -p internal/build/templates
mkdir -p internal/dns/providers
mkdir -p internal/database/engines
mkdir -p internal/vps/providers
mkdir -p internal/gitsources/providers
mkdir -p internal/webhooks/parsers
mkdir -p internal/enterprise/integrations
mkdir -p internal/db/{migrations,models}
mkdir -p web
mkdir -p marketplace/templates
mkdir -p scripts
mkdir -p docs
```

### 1.3 Entry Point

```go
// cmd/deploymonster/main.go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/deploy-monster/deploy-monster/internal/core"
)

var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    cfg, err := core.LoadConfig()
    if err != nil {
        fmt.Fprintf(os.Stderr, "config error: %v\n", err)
        os.Exit(1)
    }

    app, err := core.NewApp(cfg, core.BuildInfo{
        Version: version,
        Commit:  commit,
        Date:    date,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "init error: %v\n", err)
        os.Exit(1)
    }

    if err := app.Run(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
        os.Exit(1)
    }
}
```

---

## 2. CORE ENGINE

The core engine is the backbone — it bootstraps modules, manages lifecycle, and provides shared services.

### 2.1 Module Interface

```go
// internal/core/module.go
package core

import "context"

type HealthStatus int

const (
    HealthOK HealthStatus = iota
    HealthDegraded
    HealthDown
)

// Module is the contract every subsystem implements.
type Module interface {
    // Identity
    ID() string
    Name() string
    Version() string
    Dependencies() []string // Module IDs this depends on

    // Lifecycle
    Init(ctx context.Context, core *Core) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Observability
    Health() HealthStatus

    // HTTP integration
    Routes() []Route
    Events() []EventHandler
}

// Route represents an HTTP endpoint a module registers.
type Route struct {
    Method  string
    Path    string
    Handler HandlerFunc
    Auth    AuthLevel // None, APIKey, JWT, Admin, SuperAdmin
}

type AuthLevel int

const (
    AuthNone AuthLevel = iota
    AuthAPIKey
    AuthJWT
    AuthAdmin
    AuthSuperAdmin
)

type HandlerFunc func(ctx *RequestContext) error
```

### 2.2 Module Registry

```go
// internal/core/registry.go
package core

import (
    "context"
    "fmt"
    "sort"
    "sync"
)

type Registry struct {
    mu      sync.RWMutex
    modules map[string]Module
    order   []string // topologically sorted
}

func NewRegistry() *Registry {
    return &Registry{
        modules: make(map[string]Module),
    }
}

func (r *Registry) Register(m Module) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    id := m.ID()
    if _, exists := r.modules[id]; exists {
        return fmt.Errorf("module %q already registered", id)
    }
    r.modules[id] = m
    return nil
}

// Resolve performs topological sort based on Dependencies().
// Returns error if circular dependency detected.
func (r *Registry) Resolve() error {
    r.mu.Lock()
    defer r.mu.Unlock()

    visited := make(map[string]bool)
    visiting := make(map[string]bool)
    var sorted []string

    var visit func(id string) error
    visit = func(id string) error {
        if visited[id] {
            return nil
        }
        if visiting[id] {
            return fmt.Errorf("circular dependency detected at %q", id)
        }
        visiting[id] = true

        m, ok := r.modules[id]
        if !ok {
            return fmt.Errorf("unknown module dependency: %q", id)
        }

        for _, dep := range m.Dependencies() {
            if err := visit(dep); err != nil {
                return err
            }
        }

        visiting[id] = false
        visited[id] = true
        sorted = append(sorted, id)
        return nil
    }

    for id := range r.modules {
        if err := visit(id); err != nil {
            return err
        }
    }

    r.order = sorted
    return nil
}

// InitAll initializes modules in dependency order.
func (r *Registry) InitAll(ctx context.Context, core *Core) error {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, id := range r.order {
        m := r.modules[id]
        if err := m.Init(ctx, core); err != nil {
            return fmt.Errorf("init %s: %w", id, err)
        }
    }
    return nil
}

// StartAll starts modules in dependency order.
func (r *Registry) StartAll(ctx context.Context) error {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, id := range r.order {
        m := r.modules[id]
        if err := m.Start(ctx); err != nil {
            return fmt.Errorf("start %s: %w", id, err)
        }
    }
    return nil
}

// StopAll stops modules in reverse order.
func (r *Registry) StopAll(ctx context.Context) error {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for i := len(r.order) - 1; i >= 0; i-- {
        m := r.modules[r.order[i]]
        if err := m.Stop(ctx); err != nil {
            return fmt.Errorf("stop %s: %w", r.order[i], err)
        }
    }
    return nil
}
```

### 2.3 Event Bus

In-process, synchronous-by-default event bus. No external dependencies.

```go
// internal/core/events.go
package core

import (
    "context"
    "sync"
    "time"
)

type Event struct {
    Type      string
    Source    string    // Module ID that emitted
    Timestamp time.Time
    Data      any
}

type EventHandler struct {
    EventType string
    Handler   func(ctx context.Context, event Event) error
}

type EventBus struct {
    mu       sync.RWMutex
    handlers map[string][]func(ctx context.Context, event Event) error
}

func NewEventBus() *EventBus {
    return &EventBus{
        handlers: make(map[string][]func(ctx context.Context, event Event) error),
    }
}

func (eb *EventBus) Subscribe(eventType string, handler func(ctx context.Context, event Event) error) {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// Publish emits an event. Handlers run sequentially in the caller's goroutine.
// For async, callers wrap in `go eb.Publish(...)`.
func (eb *EventBus) Publish(ctx context.Context, event Event) error {
    eb.mu.RLock()
    handlers := eb.handlers[event.Type]
    // Also notify wildcard subscribers
    wildcards := eb.handlers["*"]
    eb.mu.RUnlock()

    event.Timestamp = time.Now()

    for _, h := range handlers {
        if err := h(ctx, event); err != nil {
            return err
        }
    }
    for _, h := range wildcards {
        if err := h(ctx, event); err != nil {
            return err
        }
    }
    return nil
}

// Standard event types
const (
    EventAppCreated       = "app.created"
    EventAppDeployed      = "app.deployed"
    EventAppStopped       = "app.stopped"
    EventAppDeleted       = "app.deleted"
    EventAppCrashed       = "app.crashed"
    EventBuildStarted     = "build.started"
    EventBuildCompleted   = "build.completed"
    EventBuildFailed      = "build.failed"
    EventDomainAdded      = "domain.added"
    EventDomainVerified   = "domain.verified"
    EventSSLIssued        = "ssl.issued"
    EventSSLExpiring      = "ssl.expiring"
    EventContainerStarted = "container.started"
    EventContainerStopped = "container.stopped"
    EventContainerDied    = "container.died"
    EventServerAdded      = "server.added"
    EventServerRemoved    = "server.removed"
    EventServerDown       = "server.down"
    EventWebhookReceived  = "webhook.received"
    EventBackupCompleted  = "backup.completed"
    EventBackupFailed     = "backup.failed"
    EventAlertTriggered   = "alert.triggered"
    EventUserCreated      = "user.created"
    EventUserLoggedIn     = "user.logged_in"
    EventTenantCreated    = "tenant.created"
    EventSecretRotated    = "secret.rotated"
    EventQuotaExceeded    = "quota.exceeded"
    EventInvoiceGenerated = "invoice.generated"
)
```

### 2.4 Core App (Orchestrator)

```go
// internal/core/app.go
package core

import (
    "context"
    "embed"
    "fmt"
    "log/slog"
    "net/http"
    "time"
)

//go:embed all:../../web/dist
var webUI embed.FS

type BuildInfo struct {
    Version string
    Commit  string
    Date    string
}

type Core struct {
    Config    *Config
    Build     BuildInfo
    Registry  *Registry
    Events    *EventBus
    DB        *Database    // Set during init by db module
    Logger    *slog.Logger
    Router    *http.ServeMux

    // Shared references — set by modules during Init()
    Docker    DockerClient    // Set by deploy module
    SSHPool   SSHPoolClient   // Set by vps module
    Secrets   SecretResolver  // Set by secrets module
}

func NewApp(cfg *Config, build BuildInfo) (*Core, error) {
    logger := slog.Default()

    c := &Core{
        Config:   cfg,
        Build:    build,
        Registry: NewRegistry(),
        Events:   NewEventBus(),
        Logger:   logger,
        Router:   http.NewServeMux(),
    }

    // Register all modules in priority order.
    // Each module package exposes a New() function.
    registerAllModules(c)

    return c, nil
}

func (c *Core) Run(ctx context.Context) error {
    c.Logger.Info("starting DeployMonster",
        "version", c.Build.Version,
        "commit", c.Build.Commit,
    )

    // 1. Resolve dependency graph
    if err := c.Registry.Resolve(); err != nil {
        return fmt.Errorf("dependency resolution: %w", err)
    }

    // 2. Init all modules (dependency order)
    if err := c.Registry.InitAll(ctx, c); err != nil {
        return fmt.Errorf("module init: %w", err)
    }

    // 3. Start all modules
    if err := c.Registry.StartAll(ctx); err != nil {
        return fmt.Errorf("module start: %w", err)
    }

    c.Logger.Info("DeployMonster is ready",
        "api", fmt.Sprintf("https://localhost:%d", c.Config.Server.Port),
    )

    // 4. Wait for shutdown signal
    <-ctx.Done()
    c.Logger.Info("shutting down...")

    // 5. Graceful shutdown (reverse order)
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    return c.Registry.StopAll(shutdownCtx)
}
```

### 2.5 Configuration

```go
// internal/core/config.go
package core

import (
    "os"
    "gopkg.in/yaml.v3"
)

type Config struct {
    Server       ServerConfig       `yaml:"server"`
    Database     DatabaseConfig     `yaml:"database"`
    Ingress      IngressConfig      `yaml:"ingress"`
    ACME         ACMEConfig         `yaml:"acme"`
    DNS          DNSConfig          `yaml:"dns"`
    Docker       DockerConfig       `yaml:"docker"`
    Backup       BackupConfig       `yaml:"backup"`
    Notifications NotificationConfig `yaml:"notifications"`
    Swarm        SwarmConfig        `yaml:"swarm"`
    VPSProviders VPSProvidersConfig `yaml:"vps_providers"`
    GitSources   GitSourcesConfig   `yaml:"git_sources"`
    Marketplace  MarketplaceConfig  `yaml:"marketplace"`
    Registration RegistrationConfig `yaml:"registration"`
    SSO          SSOConfig          `yaml:"sso"`
    Secrets      SecretsConfig      `yaml:"secrets"`
    Billing      BillingConfig      `yaml:"billing"`
    Limits       LimitsConfig       `yaml:"limits"`
    Enterprise   EnterpriseConfig   `yaml:"enterprise"`
}

type ServerConfig struct {
    Host      string `yaml:"host" env:"MONSTER_HOST" default:"0.0.0.0"`
    Port      int    `yaml:"port" env:"MONSTER_PORT" default:"8443"`
    Domain    string `yaml:"domain" env:"MONSTER_DOMAIN"`
    SecretKey string `yaml:"secret_key" env:"MONSTER_SECRET"`
}

type DatabaseConfig struct {
    Driver string `yaml:"driver" default:"sqlite"`
    Path   string `yaml:"path" default:"/var/lib/deploymonster/monster.db"`
    URL    string `yaml:"url"` // For PostgreSQL enterprise mode
}

// LoadConfig loads from monster.yaml, env vars, then defaults.
// Priority: env vars > yaml > defaults
func LoadConfig() (*Config, error) {
    cfg := &Config{}
    applyDefaults(cfg)

    // Try loading monster.yaml from standard locations
    for _, path := range []string{
        "monster.yaml",
        "/etc/deploymonster/monster.yaml",
        "/var/lib/deploymonster/monster.yaml",
    } {
        data, err := os.ReadFile(path)
        if err == nil {
            if err := yaml.Unmarshal(data, cfg); err != nil {
                return nil, fmt.Errorf("parse %s: %w", path, err)
            }
            break
        }
    }

    // Override with environment variables
    applyEnvOverrides(cfg)

    // Auto-generate secret key if not set
    if cfg.Server.SecretKey == "" {
        cfg.Server.SecretKey = generateSecretKey()
        // Persist to config file for next startup
    }

    return cfg, nil
}
```

---

## 3. DATABASE LAYER

### 3.1 SQLite (Primary, Pure Go)

Using `modernc.org/sqlite` — pure Go, no CGo, cross-compiles cleanly.

```go
// internal/db/sqlite.go
package db

import (
    "context"
    "database/sql"
    "embed"
    "fmt"
    "sort"
    "strings"

    _ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type SQLiteDB struct {
    db *sql.DB
}

func NewSQLite(path string) (*SQLiteDB, error) {
    dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL", path)
    
    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, err
    }

    // SQLite tuning for performance
    pragmas := []string{
        "PRAGMA cache_size = -64000",    // 64MB cache
        "PRAGMA mmap_size = 268435456",  // 256MB mmap
        "PRAGMA temp_store = MEMORY",
    }
    for _, p := range pragmas {
        if _, err := db.Exec(p); err != nil {
            return nil, fmt.Errorf("pragma: %w", err)
        }
    }

    // Connection pool settings
    db.SetMaxOpenConns(1)  // SQLite: single writer
    db.SetMaxIdleConns(2)

    s := &SQLiteDB{db: db}
    if err := s.migrate(); err != nil {
        return nil, fmt.Errorf("migration: %w", err)
    }

    return s, nil
}

func (s *SQLiteDB) migrate() error {
    // Create migrations table
    _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS _migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        return err
    }

    // Read migration files
    entries, err := migrationsFS.ReadDir("migrations")
    if err != nil {
        return err
    }

    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Name() < entries[j].Name()
    })

    for _, entry := range entries {
        if !strings.HasSuffix(entry.Name(), ".sql") {
            continue
        }
        // Extract version number from filename: 0001_init.sql -> 1
        var version int
        fmt.Sscanf(entry.Name(), "%04d", &version)

        // Check if already applied
        var count int
        s.db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE version = ?", version).Scan(&count)
        if count > 0 {
            continue
        }

        // Apply migration
        data, _ := migrationsFS.ReadFile("migrations/" + entry.Name())
        if _, err := s.db.Exec(string(data)); err != nil {
            return fmt.Errorf("migration %s: %w", entry.Name(), err)
        }

        s.db.Exec("INSERT INTO _migrations (version, name) VALUES (?, ?)", version, entry.Name())
    }

    return nil
}

// Tx runs a function within a transaction.
func (s *SQLiteDB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }

    if err := fn(tx); err != nil {
        tx.Rollback()
        return err
    }
    return tx.Commit()
}
```

### 3.2 Initial Migration

```sql
-- internal/db/migrations/0001_init.sql

-- Tenants (Teams)
CREATE TABLE tenants (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    avatar_url TEXT DEFAULT '',
    plan_id TEXT DEFAULT 'free',
    owner_id TEXT,
    reseller_id TEXT,
    status TEXT DEFAULT 'active' CHECK (status IN ('active','suspended','deleted')),
    limits_json TEXT DEFAULT '{}',
    metadata_json TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Users
CREATE TABLE users (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name TEXT DEFAULT '',
    avatar_url TEXT DEFAULT '',
    status TEXT DEFAULT 'active' CHECK (status IN ('active','pending','suspended','deleted')),
    totp_secret_enc TEXT,
    totp_enabled INTEGER DEFAULT 0,
    last_login_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Team Members (User ↔ Tenant link with role)
CREATE TABLE team_members (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL,
    invited_by TEXT REFERENCES users(id),
    status TEXT DEFAULT 'active' CHECK (status IN ('active','invited','removed')),
    last_active_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, user_id)
);

-- Roles
CREATE TABLE roles (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT REFERENCES tenants(id) ON DELETE CASCADE, -- NULL = built-in
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    permissions_json TEXT NOT NULL DEFAULT '[]',
    is_builtin INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Built-in roles (seeded)
INSERT INTO roles (id, name, description, permissions_json, is_builtin) VALUES
    ('role_super_admin', 'Super Admin', 'Full platform access', '["*"]', 1),
    ('role_owner', 'Owner', 'Full tenant control', '["tenant.*","app.*","project.*","member.*","billing.*","secret.*","server.*"]', 1),
    ('role_admin', 'Admin', 'Manage team and resources', '["app.*","project.*","member.*","secret.*","server.view","billing.view"]', 1),
    ('role_developer', 'Developer', 'Deploy and manage apps', '["app.*","project.view","secret.app.*","domain.*","db.*"]', 1),
    ('role_operator', 'Operator', 'Operate running apps', '["app.view","app.restart","app.logs","app.metrics"]', 1),
    ('role_viewer', 'Viewer', 'Read-only access', '["app.view","app.logs","project.view"]', 1);

-- Projects
CREATE TABLE projects (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    environment TEXT DEFAULT 'production',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Applications
CREATE TABLE applications (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT DEFAULT 'service' CHECK (type IN ('service','worker','static','database','cron','compose-stack')),
    source_type TEXT DEFAULT 'git' CHECK (source_type IN ('git','image','compose','dockerfile','marketplace')),
    source_url TEXT DEFAULT '',
    branch TEXT DEFAULT 'main',
    dockerfile TEXT DEFAULT '',
    build_pack TEXT DEFAULT '',
    env_vars_enc TEXT DEFAULT '', -- AES-256-GCM encrypted JSON
    labels_json TEXT DEFAULT '{}',
    replicas INTEGER DEFAULT 1,
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending','building','deploying','running','stopped','crashed','failed')),
    server_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Deployments
CREATE TABLE deployments (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    app_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    image TEXT DEFAULT '',
    container_id TEXT DEFAULT '',
    status TEXT DEFAULT 'pending',
    build_log TEXT DEFAULT '',
    commit_sha TEXT DEFAULT '',
    commit_message TEXT DEFAULT '',
    triggered_by TEXT DEFAULT '', -- user_id or 'webhook'
    strategy TEXT DEFAULT 'recreate',
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Domains
CREATE TABLE domains (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    app_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    fqdn TEXT NOT NULL UNIQUE,
    type TEXT DEFAULT 'custom' CHECK (type IN ('auto','custom','wildcard')),
    dns_provider TEXT DEFAULT 'manual',
    dns_synced INTEGER DEFAULT 0,
    verified INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- SSL Certificates
CREATE TABLE ssl_certs (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    cert_pem TEXT NOT NULL,
    key_pem_enc TEXT NOT NULL, -- AES-256-GCM encrypted
    issuer TEXT DEFAULT 'letsencrypt',
    expires_at DATETIME NOT NULL,
    auto_renew INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Servers
CREATE TABLE servers (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT, -- NULL = platform-level server
    hostname TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    role TEXT DEFAULT 'worker' CHECK (role IN ('manager','manager-replica','worker','worker-build','worker-db','edge')),
    provider_type TEXT DEFAULT 'custom',
    provider_ref TEXT DEFAULT '',
    ssh_port INTEGER DEFAULT 22,
    ssh_key_id TEXT,
    docker_version TEXT DEFAULT '',
    cpu_cores INTEGER DEFAULT 0,
    ram_mb INTEGER DEFAULT 0,
    disk_mb INTEGER DEFAULT 0,
    monthly_cost_cents INTEGER DEFAULT 0,
    swarm_joined INTEGER DEFAULT 0,
    agent_status TEXT DEFAULT 'unknown',
    labels_json TEXT DEFAULT '{}',
    status TEXT DEFAULT 'provisioning' CHECK (status IN ('provisioning','bootstrapping','active','maintenance','offline','destroyed')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Secrets
CREATE TABLE secrets (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT REFERENCES tenants(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
    app_id TEXT REFERENCES applications(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    type TEXT DEFAULT 'env_var',
    description TEXT DEFAULT '',
    scope TEXT DEFAULT 'app' CHECK (scope IN ('global','tenant','project','app')),
    current_version INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE secret_versions (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    value_enc TEXT NOT NULL, -- AES-256-GCM encrypted
    created_by TEXT REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(secret_id, version)
);

-- Git Sources
CREATE TABLE git_sources (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('github','gitlab','bitbucket','gitea','gogs','azure_devops','codecommit','custom_git')),
    name TEXT NOT NULL,
    base_url TEXT DEFAULT '',
    api_url TEXT DEFAULT '',
    auth_type TEXT DEFAULT 'personal_token',
    token_enc TEXT DEFAULT '',
    oauth_data_enc TEXT DEFAULT '',
    ssh_key_id TEXT,
    verified INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Webhooks
CREATE TABLE webhooks (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    app_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    git_source_id TEXT REFERENCES git_sources(id),
    secret_hash TEXT NOT NULL,
    events_json TEXT DEFAULT '["push"]',
    branch_filter TEXT DEFAULT '',
    auto_deploy INTEGER DEFAULT 1,
    status TEXT DEFAULT 'active',
    last_triggered_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE webhook_logs (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload_hash TEXT DEFAULT '',
    commit_sha TEXT DEFAULT '',
    branch TEXT DEFAULT '',
    status TEXT DEFAULT 'received',
    deployment_id TEXT,
    received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME
);

-- Managed Databases
CREATE TABLE managed_dbs (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    engine TEXT NOT NULL CHECK (engine IN ('postgres','mysql','mariadb','redis','mongodb')),
    version TEXT NOT NULL,
    port INTEGER NOT NULL,
    credentials_enc TEXT NOT NULL, -- AES-256-GCM encrypted JSON
    container_id TEXT DEFAULT '',
    volume_id TEXT DEFAULT '',
    server_id TEXT REFERENCES servers(id),
    backup_schedule TEXT DEFAULT '',
    status TEXT DEFAULT 'provisioning',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Volumes
CREATE TABLE volumes (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    app_id TEXT REFERENCES applications(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    mount_path TEXT DEFAULT '',
    size_mb INTEGER DEFAULT 0,
    driver TEXT DEFAULT 'local',
    server_id TEXT REFERENCES servers(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Backups
CREATE TABLE backups (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (source_type IN ('volume','database','config','full')),
    source_id TEXT NOT NULL,
    storage_target TEXT DEFAULT 'local',
    file_path TEXT DEFAULT '',
    size_bytes INTEGER DEFAULT 0,
    encryption TEXT DEFAULT 'aes-256-gcm',
    status TEXT DEFAULT 'pending',
    scheduled INTEGER DEFAULT 0,
    retention_days INTEGER DEFAULT 30,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- VPS Providers
CREATE TABLE vps_providers (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT, -- NULL = platform-level
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    api_token_enc TEXT NOT NULL,
    default_region TEXT DEFAULT '',
    default_size TEXT DEFAULT '',
    status TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Subscriptions (Billing)
CREATE TABLE subscriptions (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan_id TEXT NOT NULL,
    status TEXT DEFAULT 'active',
    stripe_subscription_id TEXT DEFAULT '',
    current_period_start DATETIME,
    current_period_end DATETIME,
    trial_end DATETIME,
    cancel_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Usage Records (Billing)
CREATE TABLE usage_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    app_id TEXT,
    metric_type TEXT NOT NULL, -- cpu_seconds, ram_mb_seconds, bandwidth_bytes, build_seconds, etc.
    value REAL NOT NULL,
    hour_bucket DATETIME NOT NULL, -- Rounded to hour
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_usage_tenant_hour ON usage_records(tenant_id, hour_bucket);

-- Invoices
CREATE TABLE invoices (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscription_id TEXT REFERENCES subscriptions(id),
    period_start DATETIME NOT NULL,
    period_end DATETIME NOT NULL,
    subtotal_cents INTEGER DEFAULT 0,
    tax_cents INTEGER DEFAULT 0,
    total_cents INTEGER DEFAULT 0,
    currency TEXT DEFAULT 'USD',
    status TEXT DEFAULT 'draft' CHECK (status IN ('draft','open','paid','void','uncollectible')),
    stripe_invoice_id TEXT DEFAULT '',
    pdf_url TEXT DEFAULT '',
    paid_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Audit Log
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT,
    user_id TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    details_json TEXT DEFAULT '{}',
    ip_address TEXT DEFAULT '',
    user_agent TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_audit_tenant ON audit_log(tenant_id, created_at);

-- API Keys
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    key_prefix TEXT NOT NULL, -- First 8 chars for display: "dm_xxxx..."
    scopes_json TEXT DEFAULT '["*"]',
    expires_at DATETIME,
    last_used_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Compose Stacks
CREATE TABLE compose_stacks (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    app_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    raw_yaml TEXT NOT NULL,
    parsed_json TEXT DEFAULT '{}',
    version INTEGER DEFAULT 1,
    source_type TEXT DEFAULT 'upload',
    source_url TEXT DEFAULT '',
    status TEXT DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Marketplace Installs
CREATE TABLE marketplace_installs (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    template_slug TEXT NOT NULL,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    app_id TEXT REFERENCES applications(id) ON DELETE SET NULL,
    config_json TEXT DEFAULT '{}',
    version TEXT DEFAULT '',
    status TEXT DEFAULT 'active',
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Invitations
CREATE TABLE invitations (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role_id TEXT NOT NULL,
    invited_by TEXT REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    accepted_at DATETIME,
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending','accepted','expired','revoked')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 3.3 BBolt (KV Store)

Used for sessions, cache, rate-limit counters — data that doesn't need SQL queries.

```go
// internal/db/bolt.go
package db

import (
    "encoding/json"
    "time"
    bolt "go.etcd.io/bbolt"
)

type BoltStore struct {
    db *bolt.DB
}

var (
    bucketSessions    = []byte("sessions")
    bucketRateLimit   = []byte("ratelimit")
    bucketBuildCache  = []byte("buildcache")
    bucketMetricsRing = []byte("metrics_ring")
)

func NewBoltStore(path string) (*BoltStore, error) {
    db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
    if err != nil {
        return nil, err
    }

    // Create buckets
    err = db.Update(func(tx *bolt.Tx) error {
        for _, b := range [][]byte{bucketSessions, bucketRateLimit, bucketBuildCache, bucketMetricsRing} {
            if _, err := tx.CreateBucketIfNotExists(b); err != nil {
                return err
            }
        }
        return nil
    })

    return &BoltStore{db: db}, err
}

func (b *BoltStore) Set(bucket, key string, value any, ttl time.Duration) error {
    data, err := json.Marshal(value)
    if err != nil {
        return err
    }
    entry := struct {
        Data      json.RawMessage `json:"d"`
        ExpiresAt int64           `json:"e"` // 0 = no expiry
    }{
        Data:      data,
        ExpiresAt: time.Now().Add(ttl).Unix(),
    }
    if ttl == 0 {
        entry.ExpiresAt = 0
    }
    raw, _ := json.Marshal(entry)

    return b.db.Update(func(tx *bolt.Tx) error {
        return tx.Bucket([]byte(bucket)).Put([]byte(key), raw)
    })
}

func (b *BoltStore) Get(bucket, key string, dest any) error {
    return b.db.View(func(tx *bolt.Tx) error {
        raw := tx.Bucket([]byte(bucket)).Get([]byte(key))
        if raw == nil {
            return ErrNotFound
        }
        var entry struct {
            Data      json.RawMessage `json:"d"`
            ExpiresAt int64           `json:"e"`
        }
        if err := json.Unmarshal(raw, &entry); err != nil {
            return err
        }
        if entry.ExpiresAt > 0 && time.Now().Unix() > entry.ExpiresAt {
            return ErrExpired
        }
        return json.Unmarshal(entry.Data, dest)
    })
}
```

---

## 4. AUTHENTICATION MODULE

### 4.1 JWT Implementation

```go
// internal/auth/jwt.go
package auth

import (
    "crypto/rand"
    "encoding/hex"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    jwt.RegisteredClaims
    UserID   string `json:"uid"`
    TenantID string `json:"tid"`
    RoleID   string `json:"rid"`
    Email    string `json:"email"`
}

type TokenPair struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int    `json:"expires_in"` // seconds
    TokenType    string `json:"token_type"` // "Bearer"
}

type JWTService struct {
    secretKey     []byte
    accessExpiry  time.Duration
    refreshExpiry time.Duration
}

func NewJWTService(secret string) *JWTService {
    return &JWTService{
        secretKey:     []byte(secret),
        accessExpiry:  15 * time.Minute,
        refreshExpiry: 7 * 24 * time.Hour,
    }
}

func (j *JWTService) GenerateTokenPair(userID, tenantID, roleID, email string) (*TokenPair, error) {
    now := time.Now()

    // Access token
    accessClaims := Claims{
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(now.Add(j.accessExpiry)),
            IssuedAt:  jwt.NewNumericDate(now),
            ID:        generateTokenID(),
        },
        UserID:   userID,
        TenantID: tenantID,
        RoleID:   roleID,
        Email:    email,
    }
    accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(j.secretKey)
    if err != nil {
        return nil, err
    }

    // Refresh token
    refreshClaims := jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(now.Add(j.refreshExpiry)),
        IssuedAt:  jwt.NewNumericDate(now),
        Subject:   userID,
        ID:        generateTokenID(),
    }
    refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(j.secretKey)
    if err != nil {
        return nil, err
    }

    return &TokenPair{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        ExpiresIn:    int(j.accessExpiry.Seconds()),
        TokenType:    "Bearer",
    }, nil
}

func (j *JWTService) ValidateAccessToken(tokenStr string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
        return j.secretKey, nil
    })
    if err != nil {
        return nil, err
    }
    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, ErrInvalidToken
    }
    return claims, nil
}

func generateTokenID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}
```

### 4.2 RBAC Middleware

```go
// internal/auth/middleware.go
package auth

import (
    "context"
    "encoding/json"
    "net/http"
    "strings"
)

type contextKey string

const claimsKey contextKey = "claims"

// RequireAuth extracts and validates JWT from Authorization header or cookie.
func (m *Module) RequireAuth(level AuthLevel) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            var tokenStr string

            // Try Authorization header first
            auth := r.Header.Get("Authorization")
            if strings.HasPrefix(auth, "Bearer ") {
                tokenStr = strings.TrimPrefix(auth, "Bearer ")
            }

            // Try API key
            if tokenStr == "" {
                if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
                    claims, err := m.validateAPIKey(r.Context(), apiKey)
                    if err != nil {
                        http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
                        return
                    }
                    ctx := context.WithValue(r.Context(), claimsKey, claims)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            if tokenStr == "" {
                http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
                return
            }

            claims, err := m.jwt.ValidateAccessToken(tokenStr)
            if err != nil {
                http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
                return
            }

            // Check required auth level
            if level == AuthSuperAdmin && claims.RoleID != "role_super_admin" {
                http.Error(w, `{"error":"super admin required"}`, http.StatusForbidden)
                return
            }

            ctx := context.WithValue(r.Context(), claimsKey, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// HasPermission checks if the current user has a specific permission.
func HasPermission(ctx context.Context, permission string) bool {
    claims, ok := ctx.Value(claimsKey).(*Claims)
    if !ok {
        return false
    }

    // Super admin has all permissions
    if claims.RoleID == "role_super_admin" {
        return true
    }

    // Load role permissions from DB (cached)
    role := getRoleCached(claims.RoleID)
    if role == nil {
        return false
    }

    var perms []string
    json.Unmarshal([]byte(role.PermissionsJSON), &perms)

    for _, p := range perms {
        if p == "*" || p == permission {
            return true
        }
        // Wildcard matching: "app.*" matches "app.deploy"
        if strings.HasSuffix(p, ".*") {
            prefix := strings.TrimSuffix(p, ".*")
            if strings.HasPrefix(permission, prefix+".") {
                return true
            }
        }
    }
    return false
}
```

---

## 5. API LAYER

### 5.1 Router Setup

```go
// internal/api/router.go
package api

import (
    "encoding/json"
    "net/http"
    "github.com/deploy-monster/deploy-monster/internal/core"
)

type Router struct {
    mux  *http.ServeMux
    core *core.Core
    auth *auth.Module
}

func NewRouter(c *core.Core, authMod *auth.Module) *Router {
    r := &Router{
        mux:  http.NewServeMux(),
        core: c,
        auth: authMod,
    }
    r.registerRoutes()
    return r
}

func (r *Router) registerRoutes() {
    // Health check (no auth)
    r.mux.HandleFunc("GET /health", r.handleHealth)
    r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)

    // Auth routes (no auth)
    r.mux.HandleFunc("POST /api/v1/auth/login", r.handleLogin)
    r.mux.HandleFunc("POST /api/v1/auth/register", r.handleRegister)
    r.mux.HandleFunc("POST /api/v1/auth/refresh", r.handleRefresh)

    // Webhook receiver (signature-verified, not JWT)
    r.mux.HandleFunc("POST /hooks/v1/{webhookID}", r.handleWebhook)
    r.mux.HandleFunc("POST /hooks/v1/{webhookID}/{provider}", r.handleWebhook)

    // Protected routes — registered by modules via Routes()
    r.registerModuleRoutes()

    // Static files — React SPA
    r.mux.Handle("/", r.serveSPA())
}

// JSON response helper
func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

// Error response helper
func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]string{"error": message})
}

// Pagination helper
type PaginatedResponse struct {
    Data       any   `json:"data"`
    Total      int64 `json:"total"`
    Page       int   `json:"page"`
    PerPage    int   `json:"per_page"`
    TotalPages int   `json:"total_pages"`
}
```

### 5.2 WebSocket Handler (Logs & Terminal)

```go
// internal/api/ws/logs.go
package ws

import (
    "context"
    "io"
    "net/http"

    "github.com/gorilla/websocket"
    "github.com/docker/docker/client"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

// HandleLogs streams container logs over WebSocket.
func HandleLogs(docker client.APIClient, containerID string, w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    ctx, cancel := context.WithCancel(r.Context())
    defer cancel()

    // Close WebSocket when client disconnects
    go func() {
        for {
            if _, _, err := conn.ReadMessage(); err != nil {
                cancel()
                return
            }
        }
    }()

    logReader, err := docker.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: true,
        ShowStderr: true,
        Follow:     true,
        Tail:       "100",
        Timestamps: true,
    })
    if err != nil {
        conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
        return
    }
    defer logReader.Close()

    buf := make([]byte, 4096)
    for {
        n, err := logReader.Read(buf)
        if n > 0 {
            // Docker log format: first 8 bytes are header (stream type + size)
            // Skip header, send raw text
            payload := buf[8:n]
            if len(buf) > 8 && n > 8 {
                payload = buf[8:n]
            } else {
                payload = buf[:n]
            }
            if writeErr := conn.WriteMessage(websocket.TextMessage, payload); writeErr != nil {
                return
            }
        }
        if err == io.EOF || err != nil {
            return
        }
    }
}
```

### 5.3 Embedded SPA Serving

```go
// internal/api/spa.go
package api

import (
    "embed"
    "io/fs"
    "net/http"
    "strings"
)

func (r *Router) serveSPA() http.Handler {
    // Strip "web/dist" prefix from embedded FS
    webFS, _ := fs.Sub(core.WebUI, "web/dist")
    fileServer := http.FileServer(http.FS(webFS))

    return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
        path := req.URL.Path

        // API and webhook routes are handled by other handlers
        if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/hooks/") ||
            strings.HasPrefix(path, "/ws/") || strings.HasPrefix(path, "/mcp/") {
            http.NotFound(w, req)
            return
        }

        // Try to serve the file directly
        f, err := webFS.Open(strings.TrimPrefix(path, "/"))
        if err != nil {
            // File not found — serve index.html for SPA routing
            req.URL.Path = "/"
        } else {
            f.Close()
        }

        fileServer.ServeHTTP(w, req)
    })
}
```

---

## 6. DOCKER INTEGRATION

### 6.1 Docker Client Wrapper

```go
// internal/deploy/docker.go
package deploy

import (
    "context"
    "io"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/image"
    "github.com/docker/docker/api/types/network"
    "github.com/docker/docker/client"
)

type DockerManager struct {
    client *client.Client
}

func NewDockerManager(socketPath string) (*DockerManager, error) {
    cli, err := client.NewClientWithOpts(
        client.WithHost("unix://"+socketPath),
        client.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, err
    }

    // Verify connection
    if _, err := cli.Ping(context.Background()); err != nil {
        return nil, fmt.Errorf("docker not reachable: %w", err)
    }

    return &DockerManager{client: cli}, nil
}

// CreateAndStartContainer creates a container with labels and starts it.
func (dm *DockerManager) CreateAndStartContainer(ctx context.Context, opts ContainerOpts) (string, error) {
    // Pull image if not present
    _, _, err := dm.client.ImageInspectWithRaw(ctx, opts.Image)
    if err != nil {
        reader, pullErr := dm.client.ImagePull(ctx, opts.Image, image.PullOptions{
            RegistryAuth: opts.RegistryAuth,
        })
        if pullErr != nil {
            return "", fmt.Errorf("pull %s: %w", opts.Image, pullErr)
        }
        io.Copy(io.Discard, reader) // Wait for pull to complete
        reader.Close()
    }

    // Build container config
    containerConfig := &container.Config{
        Image:  opts.Image,
        Env:    opts.EnvVars,
        Labels: opts.Labels,
    }
    if len(opts.Cmd) > 0 {
        containerConfig.Cmd = opts.Cmd
    }

    // Host config with resource limits
    hostConfig := &container.HostConfig{
        RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
        Resources: container.Resources{
            NanoCPUs: opts.CPUQuota * 1e9 / 100000, // Convert CFS quota to NanoCPUs
            Memory:   opts.MemoryLimitMB * 1024 * 1024,
            PidsLimit: &opts.PidsLimit,
        },
        PortBindings: opts.Ports,
        Binds:        opts.Volumes,
    }

    // Network config
    networkConfig := &network.NetworkingConfig{}
    if opts.NetworkID != "" {
        networkConfig.EndpointsConfig = map[string]*network.EndpointSettings{
            opts.NetworkName: {NetworkID: opts.NetworkID},
        }
    }

    resp, err := dm.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, opts.Name)
    if err != nil {
        return "", fmt.Errorf("create: %w", err)
    }

    if err := dm.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        // Cleanup on start failure
        dm.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
        return "", fmt.Errorf("start: %w", err)
    }

    return resp.ID, nil
}

type ContainerOpts struct {
    Name          string
    Image         string
    Cmd           []string
    EnvVars       []string
    Labels        map[string]string
    Ports         nat.PortMap
    Volumes       []string
    NetworkID     string
    NetworkName   string
    CPUQuota      int64
    MemoryLimitMB int64
    PidsLimit     int64
    RegistryAuth  string
}
```

### 6.2 Container Event Watcher

```go
// internal/discovery/watcher.go
package discovery

import (
    "context"
    "log/slog"

    "github.com/docker/docker/api/types/events"
    "github.com/docker/docker/api/types/filters"
    "github.com/docker/docker/client"

    "github.com/deploy-monster/deploy-monster/internal/core"
)

type Watcher struct {
    docker *client.Client
    events *core.EventBus
    logger *slog.Logger
}

func (w *Watcher) Watch(ctx context.Context) {
    filterArgs := filters.NewArgs()
    filterArgs.Add("type", "container")
    filterArgs.Add("label", "monster.enable=true")

    eventCh, errCh := w.docker.Events(ctx, events.ListOptions{
        Filters: filterArgs,
    })

    for {
        select {
        case <-ctx.Done():
            return
        case event := <-eventCh:
            switch event.Action {
            case "start":
                w.handleContainerStart(ctx, event)
            case "stop", "kill":
                w.handleContainerStop(ctx, event)
            case "die":
                w.handleContainerDie(ctx, event)
            case "health_status: healthy":
                w.handleHealthy(ctx, event)
            case "health_status: unhealthy":
                w.handleUnhealthy(ctx, event)
            }
        case err := <-errCh:
            if err != nil {
                w.logger.Error("docker events error", "err", err)
                // Reconnect after delay
            }
            return
        }
    }
}

func (w *Watcher) handleContainerStart(ctx context.Context, event events.Message) {
    labels := event.Actor.Attributes

    // Parse monster.* labels
    routerRule := labels["monster.http.routers.*.rule"]
    servicePort := labels["monster.http.services.*.loadbalancer.server.port"]

    if routerRule != "" && servicePort != "" {
        // Register in service registry → update ingress routes
        w.events.Publish(ctx, core.Event{
            Type:   core.EventContainerStarted,
            Source: "discovery",
            Data: ContainerInfo{
                ID:          event.Actor.ID,
                Labels:      labels,
                RouterRule:  routerRule,
                ServicePort: servicePort,
            },
        })
    }
}
```

---

## 7. INGRESS GATEWAY

### 7.1 Reverse Proxy Core

```go
// internal/ingress/proxy.go
package ingress

import (
    "context"
    "crypto/tls"
    "log/slog"
    "net"
    "net/http"
    "net/http/httputil"
    "net/url"
    "sync"
    "time"
)

type IngressGateway struct {
    router     *RouteTable
    httpServer *http.Server
    tlsServer  *http.Server
    acme       *ACMEManager
    logger     *slog.Logger
}

func (ig *IngressGateway) Start(ctx context.Context) error {
    // HTTP server (:80) — redirect to HTTPS + ACME challenges
    ig.httpServer = &http.Server{
        Addr:    ":80",
        Handler: ig.httpHandler(),
    }

    // HTTPS server (:443) — main TLS entrypoint
    ig.tlsServer = &http.Server{
        Addr:    ":443",
        Handler: ig.httpsHandler(),
        TLSConfig: &tls.Config{
            GetCertificate: ig.acme.GetCertificate, // Dynamic cert loading
            MinVersion:     tls.VersionTLS12,
        },
    }

    go ig.httpServer.ListenAndServe()
    go ig.tlsServer.ListenAndServeTLS("", "") // Certs from GetCertificate

    return nil
}

func (ig *IngressGateway) httpsHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Find matching route
        route := ig.router.Match(r.Host, r.URL.Path, r.Method)
        if route == nil {
            http.Error(w, "no route found", http.StatusBadGateway)
            return
        }

        // 2. Apply middleware chain
        handler := route.Handler
        for i := len(route.Middlewares) - 1; i >= 0; i-- {
            handler = route.Middlewares[i](handler)
        }

        // 3. Execute
        handler.ServeHTTP(w, r)
    })
}

// createProxy builds a reverse proxy for a backend container.
func createProxy(target string) http.Handler {
    u, _ := url.Parse("http://" + target)
    proxy := httputil.NewSingleHostReverseProxy(u)
    proxy.Transport = &http.Transport{
        DialContext: (&net.Dialer{
            Timeout:   5 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    }
    return proxy
}
```

### 7.2 Route Table

```go
// internal/ingress/router.go
package ingress

import (
    "net/http"
    "sort"
    "strings"
    "sync"
)

type RouteEntry struct {
    Host        string
    PathPrefix  string
    Priority    int
    Backends    []string // container:port targets
    LBStrategy  string
    Handler     http.Handler
    Middlewares  []func(http.Handler) http.Handler
}

type RouteTable struct {
    mu     sync.RWMutex
    routes []*RouteEntry
}

// Match finds the best route for a request.
// Priority: exact host > wildcard host > longest path prefix
func (rt *RouteTable) Match(host, path, method string) *RouteEntry {
    rt.mu.RLock()
    defer rt.mu.RUnlock()

    var best *RouteEntry

    for _, route := range rt.routes {
        // Host matching
        if !matchHost(route.Host, host) {
            continue
        }

        // Path matching
        if route.PathPrefix != "" && !strings.HasPrefix(path, route.PathPrefix) {
            continue
        }

        // Pick highest priority (or first match)
        if best == nil || route.Priority > best.Priority {
            best = route
        }
    }

    return best
}

func matchHost(pattern, host string) bool {
    if pattern == host {
        return true
    }
    // Wildcard: *.example.com matches sub.example.com
    if strings.HasPrefix(pattern, "*.") {
        suffix := pattern[1:] // .example.com
        return strings.HasSuffix(host, suffix) && strings.Count(host, ".") == strings.Count(pattern, ".")
    }
    return false
}

// Upsert adds or updates a route. Thread-safe.
func (rt *RouteTable) Upsert(entry *RouteEntry) {
    rt.mu.Lock()
    defer rt.mu.Unlock()

    // Remove existing route for same host+path
    filtered := rt.routes[:0]
    for _, r := range rt.routes {
        if r.Host != entry.Host || r.PathPrefix != entry.PathPrefix {
            filtered = append(filtered, r)
        }
    }
    filtered = append(filtered, entry)

    // Sort by priority (descending)
    sort.Slice(filtered, func(i, j int) bool {
        return filtered[i].Priority > filtered[j].Priority
    })

    rt.routes = filtered
}
```

### 7.3 Load Balancer

```go
// internal/ingress/lb/balancer.go
package lb

import (
    "net/http"
    "sync"
    "sync/atomic"
)

type Strategy interface {
    Next(backends []string, r *http.Request) string
}

// Round Robin
type RoundRobin struct {
    counter atomic.Uint64
}

func (rr *RoundRobin) Next(backends []string, r *http.Request) string {
    n := rr.counter.Add(1)
    return backends[n%uint64(len(backends))]
}

// Least Connections
type LeastConn struct {
    mu    sync.Mutex
    conns map[string]int
}

func (lc *LeastConn) Next(backends []string, r *http.Request) string {
    lc.mu.Lock()
    defer lc.mu.Unlock()

    min := int(^uint(0) >> 1)
    var best string
    for _, b := range backends {
        c := lc.conns[b]
        if c < min {
            min = c
            best = b
        }
    }
    lc.conns[best]++
    return best
}

// IP Hash (sticky sessions by client IP)
type IPHash struct{}

func (ih *IPHash) Next(backends []string, r *http.Request) string {
    ip := r.RemoteAddr
    hash := fnv32a(ip)
    return backends[hash%uint32(len(backends))]
}

func fnv32a(s string) uint32 {
    var h uint32 = 2166136261
    for i := 0; i < len(s); i++ {
        h ^= uint32(s[i])
        h *= 16777619
    }
    return h
}
```

---

## 8. SECRET VAULT

### 8.1 Encryption Engine

```go
// internal/secrets/vault.go
package secrets

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "io"

    "golang.org/x/crypto/argon2"
)

type Vault struct {
    masterKey []byte // Derived from MONSTER_SECRET via Argon2id
}

func NewVault(secretKey string) *Vault {
    // Derive 256-bit key using Argon2id
    salt := sha256.Sum256([]byte("deploymonster-vault-salt-v1"))
    key := argon2.IDKey([]byte(secretKey), salt[:16], 3, 64*1024, 4, 32)
    return &Vault{masterKey: key}
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns base64-encoded ciphertext with nonce prepended.
func (v *Vault) Encrypt(plaintext string) (string, error) {
    block, err := aes.NewCipher(v.masterKey)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }

    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
func (v *Vault) Decrypt(encoded string) (string, error) {
    data, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return "", err
    }

    block, err := aes.NewCipher(v.masterKey)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonceSize := gcm.NonceSize()
    if len(data) < nonceSize {
        return "", fmt.Errorf("ciphertext too short")
    }

    nonce, ciphertext := data[:nonceSize], data[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", err
    }

    return string(plaintext), nil
}
```

### 8.2 Secret Resolver

```go
// internal/secrets/resolver.go
package secrets

import (
    "context"
    "fmt"
    "regexp"
    "strings"
)

var secretRefPattern = regexp.MustCompile(`\$\{SECRET:([^}]+)\}`)

// ResolveEnvVars resolves all ${SECRET:name} references in environment variables.
// Scope resolution: App → Project → Tenant → Global
func (s *SecretService) ResolveEnvVars(ctx context.Context, envVars map[string]string, appID, projectID, tenantID string) (map[string]string, error) {
    resolved := make(map[string]string, len(envVars))

    for key, value := range envVars {
        resolved[key] = secretRefPattern.ReplaceAllStringFunc(value, func(match string) string {
            ref := secretRefPattern.FindStringSubmatch(match)[1]
            
            // Try scoped lookup
            val, err := s.lookupSecret(ctx, ref, appID, projectID, tenantID)
            if err != nil {
                return match // Leave unresolved if not found
            }
            return val
        })
    }

    return resolved, nil
}

func (s *SecretService) lookupSecret(ctx context.Context, name, appID, projectID, tenantID string) (string, error) {
    // Explicit scope: "global/smtp_password" or "project/db_url"
    if strings.Contains(name, "/") {
        parts := strings.SplitN(name, "/", 2)
        return s.getSecretByScope(ctx, parts[0], parts[1], appID, projectID, tenantID)
    }

    // Auto scope resolution: app → project → tenant → global
    scopes := []struct {
        scope    string
        scopeID  string
    }{
        {"app", appID},
        {"project", projectID},
        {"tenant", tenantID},
        {"global", ""},
    }

    for _, sc := range scopes {
        if sc.scopeID == "" && sc.scope != "global" {
            continue
        }
        val, err := s.getSecretValue(ctx, name, sc.scope, sc.scopeID)
        if err == nil {
            return val, nil
        }
    }

    return "", fmt.Errorf("secret %q not found in any scope", name)
}
```

---

## 9. BUILD ENGINE

### 9.1 Project Type Detection

```go
// internal/build/detector.go
package build

import (
    "os"
    "path/filepath"
)

type ProjectType string

const (
    TypeDockerfile  ProjectType = "dockerfile"
    TypeCompose     ProjectType = "compose"
    TypeNextJS      ProjectType = "nextjs"
    TypeVite        ProjectType = "vite"
    TypeNuxt        ProjectType = "nuxt"
    TypeNodeJS      ProjectType = "nodejs"
    TypeGo          ProjectType = "go"
    TypeRust        ProjectType = "rust"
    TypePython      ProjectType = "python"
    TypePHP         ProjectType = "php"
    TypeJava        ProjectType = "java"
    TypeDotNet      ProjectType = "dotnet"
    TypeRuby        ProjectType = "ruby"
    TypeStatic      ProjectType = "static"
    TypeUnknown     ProjectType = "unknown"
)

// Detect analyzes a project directory and returns the project type.
// Checks files in priority order.
func Detect(dir string) ProjectType {
    checks := []struct {
        files    []string
        projType ProjectType
    }{
        {[]string{"Dockerfile"}, TypeDockerfile},
        {[]string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}, TypeCompose},
        {[]string{"next.config.js", "next.config.mjs", "next.config.ts"}, TypeNextJS},
        {[]string{"nuxt.config.js", "nuxt.config.ts"}, TypeNuxt},
        {[]string{"vite.config.js", "vite.config.ts", "vite.config.mjs"}, TypeVite},
        {[]string{"package.json"}, TypeNodeJS},
        {[]string{"go.mod"}, TypeGo},
        {[]string{"Cargo.toml"}, TypeRust},
        {[]string{"pyproject.toml", "requirements.txt", "setup.py"}, TypePython},
        {[]string{"composer.json"}, TypePHP},
        {[]string{"pom.xml", "build.gradle", "build.gradle.kts"}, TypeJava},
        {[]string{"*.csproj", "*.sln"}, TypeDotNet},
        {[]string{"Gemfile"}, TypeRuby},
        {[]string{"index.html"}, TypeStatic},
    }

    for _, check := range checks {
        for _, file := range check.files {
            matches, _ := filepath.Glob(filepath.Join(dir, file))
            if len(matches) > 0 {
                return check.projType
            }
        }
    }

    return TypeUnknown
}
```

### 9.2 Build Pipeline

```go
// internal/build/builder.go
package build

import (
    "context"
    "fmt"
    "io"
    "os/exec"
    "time"
)

type BuildResult struct {
    Image     string
    Duration  time.Duration
    Size      int64
    Logs      string
    CommitSHA string
    Error     error
}

type Builder struct {
    docker  *deploy.DockerManager
    logger  *slog.Logger
}

// Build executes the full build pipeline:
// 1. Clone git repo (if source_type=git)
// 2. Detect project type
// 3. Generate Dockerfile (if no Dockerfile present)
// 4. Docker build
// 5. Tag and optionally push
func (b *Builder) Build(ctx context.Context, opts BuildOpts, logWriter io.Writer) (*BuildResult, error) {
    start := time.Now()
    result := &BuildResult{}

    // Step 1: Clone
    fmt.Fprintf(logWriter, "▶ Cloning %s (branch: %s)...\n", opts.RepoURL, opts.Branch)
    workDir, commitSHA, err := b.gitClone(ctx, opts)
    if err != nil {
        return nil, fmt.Errorf("clone: %w", err)
    }
    result.CommitSHA = commitSHA

    // Step 2: Detect
    projType := Detect(workDir)
    fmt.Fprintf(logWriter, "▶ Detected project type: %s\n", projType)

    // Step 3: Generate Dockerfile if needed
    dockerfilePath := filepath.Join(workDir, "Dockerfile")
    if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
        fmt.Fprintf(logWriter, "▶ No Dockerfile found, generating for %s...\n", projType)
        dockerfile := GenerateDockerfile(projType, workDir)
        if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
            return nil, fmt.Errorf("write dockerfile: %w", err)
        }
    }

    // Step 4: Docker build
    imageTag := fmt.Sprintf("monster/%s:%s", opts.AppName, commitSHA[:8])
    fmt.Fprintf(logWriter, "▶ Building image %s...\n", imageTag)

    err = b.dockerBuild(ctx, workDir, imageTag, logWriter)
    if err != nil {
        result.Error = err
        return result, fmt.Errorf("build: %w", err)
    }

    result.Image = imageTag
    result.Duration = time.Since(start)
    fmt.Fprintf(logWriter, "✅ Build completed in %s\n", result.Duration.Round(time.Millisecond))

    return result, nil
}

func (b *Builder) gitClone(ctx context.Context, opts BuildOpts) (string, string, error) {
    workDir := filepath.Join(os.TempDir(), "monster-build-"+generateID())
    
    // Clone with depth 1 for speed
    args := []string{"clone", "--depth", "1", "--branch", opts.Branch}
    
    // Handle auth
    cloneURL := opts.RepoURL
    if opts.Token != "" {
        // Inject token into HTTPS URL
        cloneURL = injectTokenInURL(opts.RepoURL, opts.Token)
    }
    
    args = append(args, cloneURL, workDir)
    
    cmd := exec.CommandContext(ctx, "git", args...)
    if output, err := cmd.CombinedOutput(); err != nil {
        return "", "", fmt.Errorf("git clone: %s: %w", string(output), err)
    }

    // Get commit SHA
    shaCmd := exec.CommandContext(ctx, "git", "-C", workDir, "rev-parse", "HEAD")
    sha, _ := shaCmd.Output()
    
    return workDir, strings.TrimSpace(string(sha)), nil
}
```

---

## 10. VPS PROVIDER INTERFACE

### 10.1 Provider Interface

```go
// internal/vps/providers/provider.go
package providers

import "context"

type Provider interface {
    // Identity
    Name() string
    Type() string // hetzner, digitalocean, vultr, linode, aws, custom

    // Discovery
    ListRegions(ctx context.Context) ([]Region, error)
    ListSizes(ctx context.Context, region string) ([]Size, error)
    ListImages(ctx context.Context) ([]Image, error)

    // Lifecycle
    CreateServer(ctx context.Context, opts CreateServerOpts) (*Server, error)
    GetServer(ctx context.Context, providerRef string) (*Server, error)
    DeleteServer(ctx context.Context, providerRef string) error
    ResizeServer(ctx context.Context, providerRef string, newSize string) error
    CreateSnapshot(ctx context.Context, providerRef string, name string) (string, error)

    // Validation
    ValidateCredentials(ctx context.Context) error
}

type Region struct {
    Slug        string `json:"slug"`
    Name        string `json:"name"`
    Country     string `json:"country"`
    Available   bool   `json:"available"`
}

type Size struct {
    Slug        string  `json:"slug"`
    Name        string  `json:"name"`
    CPUs        int     `json:"cpus"`
    MemoryMB    int     `json:"memory_mb"`
    DiskGB      int     `json:"disk_gb"`
    PriceMonthly float64 `json:"price_monthly"`
    Currency    string  `json:"currency"`
}

type CreateServerOpts struct {
    Name      string
    Region    string
    Size      string
    Image     string
    SSHKeyIDs []string
    UserData  string // cloud-init
    Labels    map[string]string
}

type Server struct {
    ProviderRef string `json:"provider_ref"` // Provider's server ID
    Name        string `json:"name"`
    IPv4        string `json:"ipv4"`
    IPv6        string `json:"ipv6"`
    Region      string `json:"region"`
    Size        string `json:"size"`
    Status      string `json:"status"`
}
```

### 10.2 Hetzner Implementation

```go
// internal/vps/providers/hetzner.go
package providers

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

const hetznerAPI = "https://api.hetzner.cloud/v1"

type Hetzner struct {
    token  string
    client *http.Client
}

func NewHetzner(token string) *Hetzner {
    return &Hetzner{
        token:  token,
        client: &http.Client{},
    }
}

func (h *Hetzner) Name() string { return "Hetzner Cloud" }
func (h *Hetzner) Type() string { return "hetzner" }

func (h *Hetzner) CreateServer(ctx context.Context, opts CreateServerOpts) (*Server, error) {
    body := map[string]any{
        "name":        opts.Name,
        "server_type": opts.Size,
        "location":    opts.Region,
        "image":       opts.Image,
        "ssh_keys":    opts.SSHKeyIDs,
        "user_data":   opts.UserData,
        "labels":      opts.Labels,
    }

    var result struct {
        Server struct {
            ID        int    `json:"id"`
            Name      string `json:"name"`
            PublicNet struct {
                IPv4 struct{ IP string } `json:"ipv4"`
                IPv6 struct{ IP string } `json:"ipv6"`
            } `json:"public_net"`
            Status string `json:"status"`
        } `json:"server"`
    }

    if err := h.doRequest(ctx, "POST", "/servers", body, &result); err != nil {
        return nil, err
    }

    return &Server{
        ProviderRef: fmt.Sprintf("%d", result.Server.ID),
        Name:        result.Server.Name,
        IPv4:        result.Server.PublicNet.IPv4.IP,
        IPv6:        result.Server.PublicNet.IPv6.IP,
        Status:      result.Server.Status,
    }, nil
}

func (h *Hetzner) doRequest(ctx context.Context, method, path string, body any, result any) error {
    var bodyReader *bytes.Buffer
    if body != nil {
        data, _ := json.Marshal(body)
        bodyReader = bytes.NewBuffer(data)
    }

    req, err := http.NewRequestWithContext(ctx, method, hetznerAPI+path, bodyReader)
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+h.token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := h.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        var errResp struct {
            Error struct{ Message string } `json:"error"`
        }
        json.NewDecoder(resp.Body).Decode(&errResp)
        return fmt.Errorf("hetzner API %d: %s", resp.StatusCode, errResp.Error.Message)
    }

    if result != nil {
        return json.NewDecoder(resp.Body).Decode(result)
    }
    return nil
}
```

---

## 11. WEBHOOK SYSTEM

### 11.1 Universal Receiver

```go
// internal/webhooks/receiver.go
package webhooks

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "net/http"

    "github.com/deploy-monster/deploy-monster/internal/webhooks/parsers"
)

type Receiver struct {
    db       *db.SQLiteDB
    dispatch chan WebhookEvent
}

type WebhookEvent struct {
    WebhookID string
    Provider  string
    Event     string
    Branch    string
    CommitSHA string
    Message   string
    Author    string
    Payload   json.RawMessage
}

func (recv *Receiver) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    webhookID := r.PathValue("webhookID")

    // Load webhook config from DB
    wh, err := recv.db.GetWebhook(r.Context(), webhookID)
    if err != nil {
        http.Error(w, "webhook not found", http.StatusNotFound)
        return
    }

    // Read body
    body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
    if err != nil {
        http.Error(w, "read error", http.StatusBadRequest)
        return
    }

    // Detect provider from headers or URL param
    provider := r.PathValue("provider")
    if provider == "" {
        provider = detectProvider(r.Header)
    }

    // Verify signature
    if !verifySignature(provider, wh.SecretHash, r.Header, body) {
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    // Parse payload using provider-specific parser
    parser := parsers.GetParser(provider)
    event, err := parser.Parse(r.Header, body)
    if err != nil {
        http.Error(w, "parse error", http.StatusBadRequest)
        return
    }

    // Check branch filter
    if wh.BranchFilter != "" && event.Branch != wh.BranchFilter {
        writeJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "branch mismatch"})
        return
    }

    // Dispatch to build/deploy pipeline
    event.WebhookID = webhookID
    recv.dispatch <- *event

    writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func verifySignature(provider, secretHash string, headers http.Header, body []byte) bool {
    switch provider {
    case "github":
        sig := headers.Get("X-Hub-Signature-256")
        if sig == "" {
            return false
        }
        mac := hmac.New(sha256.New, []byte(secretHash))
        mac.Write(body)
        expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
        return hmac.Equal([]byte(sig), []byte(expected))
    case "gitlab":
        return headers.Get("X-Gitlab-Token") == secretHash
    case "gitea":
        sig := headers.Get("X-Gitea-Signature")
        mac := hmac.New(sha256.New, []byte(secretHash))
        mac.Write(body)
        expected := hex.EncodeToString(mac.Sum(nil))
        return hmac.Equal([]byte(sig), []byte(expected))
    default:
        // Generic HMAC-SHA256
        sig := headers.Get("X-Signature-256")
        if sig == "" {
            return true // No signature = trust (configurable)
        }
        mac := hmac.New(sha256.New, []byte(secretHash))
        mac.Write(body)
        expected := hex.EncodeToString(mac.Sum(nil))
        return hmac.Equal([]byte(sig), []byte(expected))
    }
}

func detectProvider(headers http.Header) string {
    if headers.Get("X-GitHub-Event") != "" {
        return "github"
    }
    if headers.Get("X-Gitlab-Event") != "" {
        return "gitlab"
    }
    if headers.Get("X-Gitea-Event") != "" {
        return "gitea"
    }
    if headers.Get("X-Gogs-Event") != "" {
        return "gogs"
    }
    if headers.Get("X-Event-Key") != "" {
        return "bitbucket"
    }
    return "generic"
}
```

---

## 12. USAGE METERING (BILLING)

### 12.1 Metrics Collector

```go
// internal/billing/metering.go
package billing

import (
    "context"
    "time"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
)

type MeteringService struct {
    docker *client.Client
    db     *db.SQLiteDB
    ticker *time.Ticker
}

// CollectLoop runs every 60 seconds, collecting per-container metrics
// and aggregating them to hourly usage_records.
func (m *MeteringService) CollectLoop(ctx context.Context) {
    m.ticker = time.NewTicker(60 * time.Second)
    defer m.ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-m.ticker.C:
            m.collectOnce(ctx)
        }
    }
}

func (m *MeteringService) collectOnce(ctx context.Context) {
    containers, _ := m.docker.ContainerList(ctx, container.ListOptions{
        Filters: filters.NewArgs(filters.Arg("label", "monster.enable=true")),
    })

    hourBucket := time.Now().Truncate(time.Hour)

    for _, c := range containers {
        stats, err := m.docker.ContainerStatsOneShot(ctx, c.ID)
        if err != nil {
            continue
        }

        var statsJSON types.StatsJSON
        json.NewDecoder(stats.Body).Decode(&statsJSON)
        stats.Body.Close()

        // Map container → app → tenant
        appID := c.Labels["monster.app_id"]
        tenantID := c.Labels["monster.tenant_id"]
        if tenantID == "" {
            continue
        }

        // Calculate CPU seconds (delta since last collection)
        cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage-
            statsJSON.PreCPUStats.CPUUsage.TotalUsage) / 1e9 // nanoseconds → seconds

        // RAM in MB·seconds (1 minute sample)
        ramMBSeconds := float64(statsJSON.MemoryStats.Usage) / (1024 * 1024) * 60

        // Network bytes
        var rxBytes, txBytes uint64
        for _, net := range statsJSON.Networks {
            rxBytes += net.RxBytes
            txBytes += net.TxBytes
        }

        // Upsert hourly records
        m.db.UpsertUsageRecord(ctx, tenantID, appID, "cpu_seconds", cpuDelta, hourBucket)
        m.db.UpsertUsageRecord(ctx, tenantID, appID, "ram_mb_seconds", ramMBSeconds, hourBucket)
        m.db.UpsertUsageRecord(ctx, tenantID, appID, "network_tx_bytes", float64(txBytes), hourBucket)
        m.db.UpsertUsageRecord(ctx, tenantID, appID, "network_rx_bytes", float64(rxBytes), hourBucket)
    }
}
```

---

## 13. IMPLEMENTATION PATTERNS

### 13.1 Error Handling

```go
// internal/core/errors.go
package core

import "errors"

// Sentinel errors — use errors.Is() to check.
var (
    ErrNotFound       = errors.New("not found")
    ErrAlreadyExists  = errors.New("already exists")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrForbidden      = errors.New("forbidden")
    ErrQuotaExceeded  = errors.New("quota exceeded")
    ErrBuildFailed    = errors.New("build failed")
    ErrDeployFailed   = errors.New("deploy failed")
    ErrInvalidInput   = errors.New("invalid input")
    ErrExpired        = errors.New("expired")
    ErrInvalidToken   = errors.New("invalid token")
)

// AppError wraps errors with HTTP status and user-safe message.
type AppError struct {
    Code    int    `json:"code"`
    Message string `json:"error"`
    Err     error  `json:"-"` // Internal, not exposed to user
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.Err }

func NewAppError(code int, message string, err error) *AppError {
    return &AppError{Code: code, Message: message, Err: err}
}
```

### 13.2 Goroutine Patterns

```go
// Pattern: Worker Pool for concurrent builds
type BuildPool struct {
    queue   chan BuildJob
    workers int
}

func NewBuildPool(maxConcurrent int) *BuildPool {
    return &BuildPool{
        queue:   make(chan BuildJob, 100),
        workers: maxConcurrent,
    }
}

func (bp *BuildPool) Start(ctx context.Context) {
    for i := 0; i < bp.workers; i++ {
        go bp.worker(ctx, i)
    }
}

func (bp *BuildPool) worker(ctx context.Context, id int) {
    for {
        select {
        case <-ctx.Done():
            return
        case job := <-bp.queue:
            job.Execute(ctx)
        }
    }
}

// Pattern: Background ticker with graceful shutdown
func (m *MetricsCollector) Start(ctx context.Context) error {
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                m.collect(ctx)
            }
        }
    }()
    return nil
}

// Pattern: Fan-out notifications
func (n *Notifier) Send(ctx context.Context, alert Alert) {
    var wg sync.WaitGroup
    for _, channel := range alert.Channels {
        wg.Add(1)
        go func(ch NotificationChannel) {
            defer wg.Done()
            ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
            defer cancel()
            ch.Send(ctx, alert)
        }(channel)
    }
    wg.Wait()
}
```

### 13.3 Graceful Shutdown Pattern

```go
// Every long-running goroutine follows this pattern:
func (m *SomeModule) Start(ctx context.Context) error {
    // Start background goroutine
    go m.runLoop(ctx)
    return nil
}

func (m *SomeModule) Stop(ctx context.Context) error {
    // ctx has 30s timeout — clean up resources
    // Close connections, flush buffers, etc.
    return nil
}

func (m *SomeModule) runLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            // Shutdown signal received
            return
        case event := <-m.eventCh:
            m.handle(event)
        case <-m.ticker.C:
            m.periodicTask()
        }
    }
}
```

### 13.4 Testing Strategy

```go
// Use interfaces + mocks for testability.
// Example: Mock Docker client for unit tests.

type DockerClient interface {
    ContainerCreate(ctx context.Context, config *container.Config, ...) (container.CreateResponse, error)
    ContainerStart(ctx context.Context, id string, opts container.StartOptions) error
    ContainerStop(ctx context.Context, id string, opts container.StopOptions) error
    ContainerList(ctx context.Context, opts container.ListOptions) ([]types.Container, error)
    ContainerLogs(ctx context.Context, id string, opts container.LogsOptions) (io.ReadCloser, error)
    // ... only methods we actually use
}

// In tests:
type mockDocker struct {
    containers map[string]*container.Config
}

func (m *mockDocker) ContainerCreate(...) (container.CreateResponse, error) {
    // Return predictable test data
}
```

---

## 14. REACT UI BUILD INTEGRATION

### 14.1 Embed Strategy

```go
// internal/core/app.go (near the top)

import "embed"

// This embeds the entire compiled React app into the Go binary.
// The build script must run `cd web && npm run build` before `go build`.
//
//go:embed all:../../web/dist
var webUI embed.FS
```

### 14.2 Build Script

```bash
#!/bin/bash
# scripts/build.sh

set -euo pipefail

VERSION=${VERSION:-dev}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

echo "▶ Building React UI..."
cd web
npm ci --silent
npm run build
cd ..

echo "▶ Building Go binary..."
CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o bin/deploymonster \
    ./cmd/deploymonster

echo "✅ Build complete: bin/deploymonster ($(du -h bin/deploymonster | cut -f1))"
```

### 14.3 Makefile

```makefile
.PHONY: build dev test clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	bash scripts/build.sh

dev:
	# Run Go backend with hot reload (using air or similar)
	air -c .air.toml

dev-ui:
	cd web && npm run dev

test:
	go test ./... -race -count=1

test-coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ web/dist/ coverage.out coverage.html

docker:
	docker build -t deploymonster/deploymonster:$(VERSION) .

release:
	goreleaser release --clean
```

---

## 15. CROSS-CUTTING CONCERNS

### 15.1 Structured Logging

```go
// All modules use slog with consistent fields:
logger.Info("container started",
    "module", "deploy",
    "app_id", appID,
    "container_id", containerID,
    "tenant_id", tenantID,
    "image", image,
    "duration_ms", duration.Milliseconds(),
)

// Secret masking in logs
logger.Info("env vars resolved",
    "app_id", appID,
    "var_count", len(envVars),
    // NEVER log actual secret values
)
```

### 15.2 Audit Logging

```go
// Every state-changing action is audit-logged:
func (a *AuditService) Log(ctx context.Context, entry AuditEntry) {
    claims := auth.ClaimsFromContext(ctx)
    
    a.db.InsertAuditLog(ctx, db.AuditLog{
        TenantID:     claims.TenantID,
        UserID:       claims.UserID,
        Action:       entry.Action,
        ResourceType: entry.ResourceType,
        ResourceID:   entry.ResourceID,
        DetailsJSON:  entry.Details,
        IPAddress:    entry.IP,
        UserAgent:    entry.UserAgent,
    })
}

// Usage in handlers:
audit.Log(ctx, AuditEntry{
    Action:       "app.deployed",
    ResourceType: "application",
    ResourceID:   appID,
    Details:      `{"version": 12, "commit": "abc123", "strategy": "rolling"}`,
})
```

### 15.3 Rate Limiting

```go
// internal/api/middleware/ratelimit.go
// Token bucket per IP, stored in BBolt.
type RateLimiter struct {
    store  *db.BoltStore
    limit  int // requests per window
    window time.Duration
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := realIP(r)
        key := "rl:" + ip

        var counter int
        rl.store.Get("ratelimit", key, &counter)

        if counter >= rl.limit {
            w.Header().Set("Retry-After", fmt.Sprintf("%d", int(rl.window.Seconds())))
            http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
            return
        }

        rl.store.Set("ratelimit", key, counter+1, rl.window)
        next.ServeHTTP(w, r)
    })
}
```

---

## 16. DEPLOYMENT ORDER

When implementing, follow this strict order to always have a working, testable system:

```
Phase 1 — Foundation
  1. cmd/deploymonster/main.go (entry point)
  2. internal/core/ (config, module system, event bus)
  3. internal/db/ (SQLite + migrations + BBolt)
  4. internal/auth/ (JWT + RBAC middleware)
  5. internal/api/ (router, health endpoint, SPA serving)
  6. internal/deploy/ (Docker client wrapper, container CRUD)
  7. web/ (React shell: login page, basic dashboard)
  → Milestone: Login → see container list → start/stop containers

Phase 2 — Ingress
  8. internal/ingress/ (reverse proxy, route table)
  9. internal/ingress/lb/ (round-robin first)
  10. internal/discovery/ (Docker event watcher, label parser)
  11. internal/ingress/acme.go (Let's Encrypt via lego)
  → Milestone: Deploy container with monster.* labels → auto-routed with SSL

Phase 3 — Build & Git
  12. internal/build/ (detector, dockerfile templates, builder)
  13. internal/gitsources/ (git clone, provider interface)
  14. internal/gitsources/providers/ (GitHub first, then others)
  15. internal/webhooks/ (receiver, signature verify, parsers)
  → Milestone: Push to GitHub → webhook → auto-build → deploy → live

Phase 4 — Full Feature Set
  16. internal/secrets/ (vault, encryption, resolver)
  17. internal/compose/ (YAML parser, multi-service deploy)
  18. internal/dns/ (Cloudflare sync)
  19. internal/resource/ (metrics collector, alerts)
  20. internal/backup/ (volume tar, S3 upload)
  21. internal/database/ (PG/MySQL/Redis provisioning)
  22. internal/marketplace/ (template loader, config wizard)
  23. internal/vps/ (Hetzner first, then others)
  24. internal/swarm/ (multi-node orchestration)
  25. internal/billing/ (plans, metering, Stripe)
  26. internal/enterprise/ (white-label, reseller)
  27. internal/mcp/ (MCP server for AI control)
```

---

## 17. PERFORMANCE TARGETS

| Metric | Target | How to Achieve |
|--------|--------|----------------|
| Binary size | < 50 MB | `-ldflags="-s -w"`, UPX optional |
| Startup time | < 3s | Lazy module init, no heavy startup work |
| API latency (p95) | < 100ms | SQLite WAL mode, in-memory caching |
| Proxy overhead | < 5ms | `httputil.ReverseProxy`, keep-alive pools |
| Memory (idle) | < 100 MB | Minimal goroutines, no global caches |
| SQLite write | < 1ms | WAL mode, prepared statements |
| Docker API | < 50ms | Unix socket, connection reuse |
| Build start | < 10s | Pre-pulled base images, layer cache |
| Cert issuance | < 60s | Async ACME with retry |

---

*This implementation guide is the companion to SPECIFICATION.md. Every code pattern here maps to a feature in the spec. When in doubt, the spec is the source of truth for what to build; this document is the source of truth for how to build it.*
