# Deployment Guide

## Production Deployment

### System Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 512 MB | 2 GB |
| Disk | 10 GB | 50 GB |
| OS | Ubuntu 22.04+ | Ubuntu 24.04 |
| Docker | 24.0+ | Latest |

### Installation

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
```

### Systemd Service

The installer creates a systemd service automatically:

```bash
sudo systemctl start deploymonster
sudo systemctl enable deploymonster
sudo systemctl status deploymonster
```

### Configuration

```bash
deploymonster init  # Creates monster.yaml
vim monster.yaml    # Edit settings
sudo systemctl restart deploymonster
```

## Custom Domains

1. Add domain via UI: **Domains** → **Add Domain**
2. Point DNS A record to your server IP:
   ```
   app.example.com  A  → 1.2.3.4
   ```
3. SSL certificate is auto-provisioned via Let's Encrypt

### Auto-Subdomains

Configure a wildcard DNS record:
```
*.deploy.example.com  A  → 1.2.3.4
```

Set in config:
```yaml
dns:
  auto_subdomain: deploy.example.com
```

Apps automatically get `{app-name}.deploy.example.com`.

## Git Providers

### GitHub
1. Go to **Settings** → create a Personal Access Token
2. In DeployMonster: **Git** → connect GitHub with token

### GitLab
1. Go to **Settings** → create a Personal Access Token
2. In DeployMonster: **Git** → connect GitLab with token

### Auto-Deploy
When you connect a Git source, DeployMonster auto-creates a webhook. Every push triggers: clone → build → deploy.

## Backups

### Automatic Backups

```yaml
backup:
  schedule: "02:00"       # Daily at 2 AM
  retention_days: 30
  storage_path: /var/lib/deploymonster/backups
```

### S3 Storage

```yaml
backup:
  storage:
    type: s3
    endpoint: https://s3.amazonaws.com
    bucket: my-backups
    region: eu-central-1
    access_key: ${SECRET:aws_key}
    secret_key: ${SECRET:aws_secret}
```

### Manual Backup

API: `POST /api/v1/backups`

## Team Management

### Invite Members
1. Go to **Team** → **Invite**
2. Enter email and select role
3. Member receives invite link

### Built-in Roles

| Role | Permissions |
|------|------------|
| Super Admin | Everything |
| Owner | Full tenant control |
| Admin | Manage team and resources |
| Developer | Deploy and manage apps |
| Operator | Restart, view logs |
| Viewer | Read-only |

## Notifications

### Slack
```yaml
notifications:
  slack_webhook: https://hooks.slack.com/services/...
```

### Discord
```yaml
notifications:
  discord_webhook: https://discord.com/api/webhooks/...
```

### Telegram
```yaml
notifications:
  telegram_token: 123456:ABC-DEF
```

## Monitoring

### Prometheus
Metrics available at `GET /metrics`:
- `deploymonster_uptime_seconds`
- `deploymonster_go_goroutines`
- `deploymonster_module_health`
- `deploymonster_events_published_total`
- `deploymonster_containers_total`

### Health Check
`GET /health` returns module status:
```json
{"status": "ok", "version": "0.1.0", "modules": {"core.db": "ok", ...}}
```

## Scaling

### Multi-Server (Docker Swarm)

1. Initialize swarm on master:
   ```bash
   deploymonster serve
   ```

2. Add worker nodes:
   - Go to **Servers** → **Add Server**
   - Select provider (Hetzner, DO, Vultr, Linode)
   - Or connect existing server via SSH

3. DeployMonster bootstraps the server: installs Docker, deploys agent, joins swarm.

## Security

### SSL/TLS
- Auto-provisioned via Let's Encrypt (HTTP-01 challenge)
- TLS 1.2 minimum, TLS 1.3 preferred
- Wildcard certs via DNS-01 (Cloudflare)

### Secrets
- AES-256-GCM encryption at rest
- Argon2id key derivation
- Scoped: global → tenant → project → app
- `${SECRET:name}` syntax in env vars and compose files

### 2FA
- TOTP (Google Authenticator, Authy)
- 8 recovery codes generated on setup
