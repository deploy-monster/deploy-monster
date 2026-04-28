-- DeployMonster initial schema
-- Version: 0001

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

-- Team Members (User <-> Tenant link with role)
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
    tenant_id TEXT REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    permissions_json TEXT NOT NULL DEFAULT '[]',
    is_builtin INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Built-in roles
INSERT INTO roles (id, name, description, permissions_json, is_builtin) VALUES
    ('role_super_admin', 'Super Admin', 'Full platform access', '["*"]', 1),
    ('role_owner', 'Owner', 'Full tenant control', '["tenant.*","app.*","project.*","member.*","billing.*","secret.*","server.*","domain.*","db.*"]', 1),
    ('role_admin', 'Admin', 'Manage team and resources', '["app.*","project.*","member.*","secret.*","server.*","billing.*","domain.*","db.*"]', 1),
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
    env_vars_enc TEXT DEFAULT '',
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
    triggered_by TEXT DEFAULT '',
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
    key_pem_enc TEXT NOT NULL,
    issuer TEXT DEFAULT 'letsencrypt',
    expires_at DATETIME NOT NULL,
    auto_renew INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Servers
CREATE TABLE servers (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    tenant_id TEXT,
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
    value_enc TEXT NOT NULL,
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
    credentials_enc TEXT NOT NULL,
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
    tenant_id TEXT,
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
    metric_type TEXT NOT NULL,
    value REAL NOT NULL,
    hour_bucket DATETIME NOT NULL,
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
    key_prefix TEXT NOT NULL,
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
