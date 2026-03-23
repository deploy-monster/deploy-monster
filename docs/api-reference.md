# API Reference

Base URL: `https://your-server:8443/api/v1`

Authentication: `Authorization: Bearer <access_token>` or `X-API-Key: dm_xxxxx`

## Authentication

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/auth/login` | None | Login with email + password |
| POST | `/auth/register` | None | Register new user + tenant |
| POST | `/auth/refresh` | None | Refresh access token |
| GET | `/auth/me` | JWT | Get current user profile |
| PATCH | `/auth/me` | JWT | Update profile (name, avatar) |
| POST | `/auth/change-password` | JWT | Change password |

## Applications

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/apps` | JWT | List apps (paginated) |
| POST | `/apps` | JWT | Create app |
| GET | `/apps/{id}` | JWT | Get app details |
| PATCH | `/apps/{id}` | JWT | Update app |
| DELETE | `/apps/{id}` | JWT | Delete app |
| POST | `/apps/{id}/start` | JWT | Start app |
| POST | `/apps/{id}/stop` | JWT | Stop app |
| POST | `/apps/{id}/restart` | JWT | Restart app |
| POST | `/apps/{id}/deploy` | JWT | Trigger build+deploy |
| POST | `/apps/{id}/scale` | JWT | Set replica count |
| POST | `/apps/{id}/rollback` | JWT | Rollback to version |
| GET | `/apps/{id}/versions` | JWT | List deployment versions |
| GET | `/apps/{id}/stats` | JWT | Container stats |
| GET | `/apps/{id}/logs` | JWT | Get log lines |
| GET | `/apps/{id}/logs/stream` | JWT | SSE log stream |
| GET | `/apps/{id}/env` | JWT | Get env vars (masked) |
| PUT | `/apps/{id}/env` | JWT | Update env vars |
| POST | `/apps/{id}/exec` | JWT | Execute command |
| GET | `/apps/{id}/terminal` | JWT | SSE terminal output |
| POST | `/apps/{id}/terminal` | JWT | Send terminal command |
| GET | `/apps/{id}/deployments` | JWT | List deployments |
| GET | `/apps/{id}/deployments/latest` | JWT | Latest deployment |

## Projects

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/projects` | JWT | List projects |
| POST | `/projects` | JWT | Create project |
| GET | `/projects/{id}` | JWT | Get project |
| DELETE | `/projects/{id}` | JWT | Delete project |

## Domains

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/domains` | JWT | List domains |
| POST | `/domains` | JWT | Add domain |
| DELETE | `/domains/{id}` | JWT | Remove domain |

## Databases

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/databases/engines` | None | List available engines |
| POST | `/databases` | JWT | Create managed database |

## Backups

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/backups` | JWT | List backups |
| POST | `/backups` | JWT | Create backup |
| GET | `/backups/{key}/download` | JWT | Download backup |

## Compose Stacks

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/stacks` | JWT | Deploy compose YAML |
| POST | `/stacks/validate` | JWT | Validate compose YAML |

## Secrets

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/secrets` | JWT | List secrets (names only) |
| POST | `/secrets` | JWT | Create encrypted secret |

## Volumes

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/volumes` | JWT | List volumes |
| POST | `/volumes` | JWT | Create volume |

## Servers

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/servers/providers` | JWT | List VPS providers |
| GET | `/servers/providers/{p}/regions` | JWT | List regions |
| GET | `/servers/providers/{p}/sizes` | JWT | List sizes |
| POST | `/servers/provision` | JWT | Provision server |
| GET | `/servers/stats` | JWT | Server statistics |

## Git Sources

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/git/providers` | JWT | List connected providers |
| GET | `/git/{provider}/repos` | JWT | List repositories |
| GET | `/git/{provider}/repos/{repo}/branches` | JWT | List branches |

## Team

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/team/roles` | JWT | List roles |
| GET | `/team/audit-log` | JWT | View audit log |
| POST | `/team/invites` | JWT | Send invitation |

## Marketplace

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/marketplace` | None | List templates |
| GET | `/marketplace/{slug}` | None | Get template |
| POST | `/marketplace/deploy` | JWT | Deploy template |

## Billing

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/billing/plans` | None | List plans |
| GET | `/billing/usage` | JWT | Current usage + quota |

## Notifications

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/notifications/test` | JWT | Send test notification |

## Admin

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/admin/system` | JWT | System info |
| PATCH | `/admin/settings` | JWT | Update settings |
| GET | `/admin/tenants` | JWT | List all tenants |
| PATCH | `/admin/branding` | JWT | Update branding |

## Branding

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/branding` | None | Get platform branding |

## MCP (Model Context Protocol)

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/mcp/v1/tools` | None | List MCP tools |
| POST | `/mcp/v1/tools/{name}` | None | Call MCP tool |

## Webhooks

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/hooks/v1/{webhookID}` | Signature | Receive webhook |

## System

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/health` | None | Health check |
| GET | `/metrics` | None | Prometheus metrics |
| GET | `/events/stream` | JWT | SSE event stream |
