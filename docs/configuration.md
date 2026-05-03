# Configuration Reference

DeployMonster is configured via a YAML file (`monster.yaml`) with environment variable overrides.

**Priority order:** Environment variables > YAML file > Defaults

**Config file locations** (checked in order):
1. Path given via `--config` CLI flag
2. `./monster.yaml` (current directory)
3. `/etc/deploymonster/monster.yaml`
4. `/var/lib/deploymonster/monster.yaml`

## Environment Variables

All environment variables use the `MONSTER_` prefix.

| Variable | YAML Path | Default | Description |
|----------|-----------|---------|-------------|
| `MONSTER_HOST` | `server.host` | `0.0.0.0` | HTTP server bind address |
| `MONSTER_PORT` | `server.port` | `8443` | HTTP server port (1-65535) |
| `MONSTER_DOMAIN` | `server.domain` | _(empty)_ | Public domain (used for CORS, cookies) |
| `MONSTER_SECRET` | `server.secret_key` | _(auto-generated)_ | JWT signing key (min 32 chars) |
| `MONSTER_PREVIOUS_SECRET_KEYS` | `server.previous_secret_keys` | _(empty)_ | Comma-separated old keys for JWT rotation |
| `MONSTER_CORS_ORIGINS` | `server.cors_origins` | _(derived from domain)_ | Comma-separated allowed CORS origins |
| `MONSTER_RATE_LIMIT_PER_MINUTE` | `server.rate_limit_per_minute` | `120` | Global per-IP API/hook rate limit; `0` disables it |
| `MONSTER_ENABLE_PPROF` | `server.enable_pprof` | `false` | Enable `/debug/pprof/*` endpoints (auth-protected) |
| `MONSTER_DB_PATH` | `database.path` | `deploymonster.db` | SQLite database file path |
| `MONSTER_DB_URL` | `database.url` | _(empty)_ | PostgreSQL connection URL (switches driver to postgres) |
| `MONSTER_DOCKER_HOST` | `docker.host` | `unix:///var/run/docker.sock` | Docker daemon socket/host |
| `MONSTER_ACME_EMAIL` | `acme.email` | _(empty)_ | Email for Let's Encrypt certificate registration |
| `MONSTER_REGISTRATION_MODE` | `registration.mode` | `open` | User registration mode (see below) |
| `MONSTER_LOG_LEVEL` | `server.log_level` | `info` | Log level (debug, info, warn, error) |
| `MONSTER_LOG_FORMAT` | `server.log_format` | `text` | Log format: `text` (human-readable) or `json` (structured) |
| `MONSTER_ADMIN_EMAIL` | — | `admin@deploymonster.local` | Initial admin email (first-run setup only) |
| `MONSTER_ADMIN_PASSWORD` | — | _(auto-generated and logged once)_ | Initial admin password (first-run setup only) |

## YAML Configuration Sections

### server

```yaml
server:
  host: "0.0.0.0"           # Bind address
  port: 8443                 # HTTP port (validated: 1-65535)
  domain: "deploy.example.com" # Public domain
  secret_key: ""             # JWT signing key (auto-generated if empty, min 32 chars)
  previous_secret_keys: []   # Old signing keys for graceful JWT rotation
  cors_origins: ""           # Comma-separated origins (auto-derived from domain if empty)
  rate_limit_per_minute: 120 # Global per-IP API/hook limit; 0 disables
  enable_pprof: false        # Enable Go pprof profiling endpoints
  log_level: "info"          # debug, info, warn, error
  log_format: "text"         # text (human-readable) or json (structured, for log aggregators)
```

### database

```yaml
database:
  driver: "sqlite"           # Database driver: "sqlite" or "postgres"
  path: "deploymonster.db"   # SQLite file path (required when driver=sqlite)
  url: ""                    # PostgreSQL URL (required when driver=postgres)
```

### ingress

```yaml
ingress:
  http_port: 80              # Ingress HTTP port (validated: 1-65535)
  https_port: 443            # Ingress HTTPS port (validated: 1-65535)
  enable_https: true         # Enable HTTPS with auto-cert
  force_https: true          # Redirect HTTP traffic to HTTPS
```

### acme

```yaml
acme:
  email: ""                  # Let's Encrypt registration email
  staging: false             # Use Let's Encrypt staging environment
  cert_dir: ""               # Certificate storage directory
  provider: "http-01"        # Challenge provider: "http-01" or "dns-01"
```

### dns

```yaml
dns:
  provider: ""               # DNS provider: "cloudflare", "route53", "manual"
  cloudflare_token: ""       # Cloudflare API token
  auto_subdomain: ""         # Auto-subdomain base (e.g., "deploy.monster")
```

### docker

```yaml
docker:
  host: "unix:///var/run/docker.sock"  # Docker socket or TCP host
  api_version: ""            # Docker API version override
  tls_verify: false          # Verify TLS for TCP connections
```

### backup

```yaml
backup:
  schedule: ""               # Daily backup time (HH:MM format, default: 02:00)
  retention_days: 30         # Days to keep backups
  storage_path: "/var/lib/deploymonster/backups"  # Local backup storage
  encryption: true           # Encrypt backups at rest
  s3:                        # S3-compatible storage (optional, registers alongside local)
    bucket: ""               # S3 bucket name (empty = S3 disabled)
    region: "us-east-1"      # AWS region
    endpoint: ""             # Custom endpoint for MinIO/R2 (empty = AWS default)
    access_key: ""           # AWS access key ID
    secret_key: ""           # AWS secret access key
    path_style: false        # Use path-style URLs (required for MinIO)
```

### notifications

```yaml
notifications:
  slack_webhook: ""          # Slack webhook URL
  discord_webhook: ""        # Discord webhook URL
  telegram_token: ""         # Telegram bot token
```

### swarm

```yaml
swarm:
  enabled: false             # Enable Docker Swarm mode
  manager_ip: ""             # Swarm manager IP address
  join_token: ""             # Swarm join token for workers
```

### vps_providers

```yaml
vps_providers:
  enabled: false             # Enable VPS provisioning
```

### git_sources

```yaml
git_sources:
  github_client_id: ""       # GitHub OAuth app client ID
  github_client_secret: ""   # GitHub OAuth app client secret
  gitlab_client_id: ""       # GitLab OAuth app client ID
  gitlab_client_secret: ""   # GitLab OAuth app client secret
```

### marketplace

```yaml
marketplace:
  enabled: true              # Enable built-in marketplace templates
  templates_dir: "marketplace/templates"  # Template directory
  community_sync: false      # Sync community templates
```

Marketplace templates can auto-generate missing sensitive config values, such
as database passwords, during deploy. Generated values are returned only in the
deploy response under `generated_secrets`; they are not persisted in plaintext
or retrievable later. Save them in your password manager or secret-management
process before leaving the deployment confirmation screen.

### registration

```yaml
registration:
  mode: "open"               # Registration mode (validated)
```

**Valid registration modes:**
- `open` — Anyone can register
- `invite_only` — Registration requires an invitation
- `approval` — Registration requires admin approval
- `disabled` — Registration is disabled

### secrets

```yaml
secrets:
  encryption_key: ""         # AES-256-GCM encryption key for secret vault
```

### billing

```yaml
billing:
  enabled: false             # Enable billing/subscription features
  stripe_secret_key: ""      # Stripe API secret key
  stripe_webhook_key: ""     # Stripe webhook signing secret
```

### limits

```yaml
limits:
  max_apps_per_tenant: 100   # Max apps per tenant (validated: >= 0)
  max_build_minutes: 30      # Max build duration in minutes
  max_concurrent_builds: 5   # Max concurrent builds (validated: >= 1)
```

### enterprise

```yaml
enterprise:
  enabled: false             # Enable enterprise features
  license_key: ""            # Enterprise license key
```

## Startup Validation

The configuration is validated during `LoadConfig()`. The following rules are enforced:

| Rule | Error |
|------|-------|
| Port must be 1-65535 | `config: server.port N out of range` |
| Secret key must be >= 32 chars | `config: server.secret_key must be at least 32 characters` |
| Database driver must be `sqlite` or `postgres` | `config: unsupported database.driver` |
| SQLite requires `database.path` | `config: database.path is required for sqlite driver` |
| PostgreSQL requires `database.url` | `config: database.url is required for postgres driver` |
| Ingress ports must be 1-65535 | `config: ingress.http_port N out of range` |
| Registration mode must be valid | `config: registration.mode not recognized` |
| Max apps per tenant must be >= 0 | `config: limits.max_apps_per_tenant must be non-negative` |
| Max concurrent builds must be >= 1 | `config: limits.max_concurrent_builds must be at least 1` |

## Minimal Production Configuration

```yaml
server:
  domain: "deploy.example.com"
  secret_key: "your-secret-key-at-least-32-characters"

ingress:
  force_https: true

acme:
  email: "admin@example.com"
```

Everything else uses sensible defaults. The secret key is auto-generated on first run if not specified.

## Hot Reload

DeployMonster supports config hot-reload via the `SIGHUP` signal. Safe-to-reload fields are applied without restart:

| Reloadable Field | YAML Path |
|------------------|-----------|
| Log level | `server.log_level` |
| Log format | `server.log_format` |
| CORS origins | `server.cors_origins` |
| Registration mode | `registration.mode` |
| Backup schedule | `backup.schedule` |
| Max apps per tenant | `limits.max_apps_per_tenant` |
| Max concurrent builds | `limits.max_concurrent_builds` |

Fields that require restart (port, database, Docker host, secret key) are **not** changed on reload.

```bash
# Reload configuration
kill -SIGHUP $(pidof deploymonster)

# Or if running via systemd
systemctl reload deploymonster
```

A `system.config_reloaded` event is published on successful reload, containing the list of changed fields.

## Log Rotation

DeployMonster writes logs to stdout/stderr. Use your platform's native log management to handle rotation and retention.

### systemd (recommended for bare-metal)

```ini
# /etc/systemd/system/deploymonster.service
[Service]
ExecStart=/usr/local/bin/deploymonster
StandardOutput=journal
StandardError=journal
SyslogIdentifier=deploymonster
```

Query logs:
```bash
journalctl -u deploymonster --since "1 hour ago"
journalctl -u deploymonster -f              # follow
journalctl -u deploymonster -o json         # JSON output for parsing
```

Configure retention in `/etc/systemd/journald.conf`:
```ini
[Journal]
SystemMaxUse=500M
MaxRetentionSec=30day
```

### logrotate (traditional)

If running with output redirected to a file:

```
# /etc/logrotate.d/deploymonster
/var/log/deploymonster/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
```

### Docker

When running DeployMonster in a container, configure the Docker logging driver:

```bash
docker run -d \
  --log-driver json-file \
  --log-opt max-size=50m \
  --log-opt max-file=5 \
  ghcr.io/deploy-monster/deploy-monster:latest
```

Or set defaults in `/etc/docker/daemon.json`:
```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "50m",
    "max-file": "5"
  }
}
```

### Structured JSON logs

For log aggregators (ELK, Loki, Datadog), enable JSON format:

```yaml
server:
  log_format: "json"
```

Or via environment variable:
```bash
export MONSTER_LOG_FORMAT=json
```

Each log line includes `time`, `level`, `msg`, `module`, and contextual fields like `request_id` and `correlation_id`.
