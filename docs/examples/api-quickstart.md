# DeployMonster API — Quick Start

Base URL: `https://your-domain:8443/api/v1`

## Authentication

```bash
# Register
curl -X POST /api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"SecurePass123!","name":"Admin"}'

# Login
curl -X POST /api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"SecurePass123!"}'

# Response: { "data": { "access_token": "eyJ...", "refresh_token": "eyJ..." } }

# Use token in subsequent requests
export TOKEN="eyJ..."
```

## Deploy an App (Git Source)

```bash
# Create app from GitHub
curl -X POST /api/v1/apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-api",
    "source_type": "git",
    "git_url": "https://github.com/user/repo.git",
    "branch": "main"
  }'

# Trigger deploy
curl -X POST /api/v1/apps/{app_id}/deploy \
  -H "Authorization: Bearer $TOKEN"
```

## Deploy from Docker Image

```bash
curl -X POST /api/v1/apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis-cache",
    "source_type": "image",
    "image": "redis:7-alpine"
  }'
```

## Deploy from Marketplace

```bash
# List templates
curl /api/v1/marketplace \
  -H "Authorization: Bearer $TOKEN"

# Deploy a template
curl -X POST /api/v1/marketplace/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug": "wordpress", "name": "my-blog"}'
```

If the template generates missing sensitive values, the response includes a
one-time `generated_secrets` object. Save those values before closing the
response; they are not retrievable later.

## Manage Apps

```bash
# List all apps
curl /api/v1/apps -H "Authorization: Bearer $TOKEN"

# Get app details
curl /api/v1/apps/{app_id} -H "Authorization: Bearer $TOKEN"

# Start / Stop / Restart
curl -X POST /api/v1/apps/{app_id}/start -H "Authorization: Bearer $TOKEN"
curl -X POST /api/v1/apps/{app_id}/stop -H "Authorization: Bearer $TOKEN"
curl -X POST /api/v1/apps/{app_id}/restart -H "Authorization: Bearer $TOKEN"

# Scale
curl -X POST /api/v1/apps/{app_id}/scale \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"replicas": 3}'

# Delete
curl -X DELETE /api/v1/apps/{app_id} -H "Authorization: Bearer $TOKEN"
```

## Domains & SSL

```bash
# Add domain
curl -X POST /api/v1/domains \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"app_id": "app_123", "fqdn": "myapp.example.com"}'

# SSL is auto-provisioned via Let's Encrypt
```

## Environment Variables

```bash
# Set env vars
curl -X PUT /api/v1/apps/{app_id}/env \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"DATABASE_URL": "postgres://...", "REDIS_URL": "redis://..."}'

# Import .env file
curl -X POST /api/v1/apps/{app_id}/env/import \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: text/plain" \
  -d 'DATABASE_URL=postgres://...
REDIS_URL=redis://...'
```

## Databases

```bash
# Create managed PostgreSQL
curl -X POST /api/v1/databases \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"engine": "postgres", "name": "mydb", "version": "16"}'
```

## Webhooks

```bash
# Configure webhook endpoint in GitHub/GitLab
# URL: https://your-domain:8443/api/v1/webhooks/{app_id}
# Secret: your-webhook-secret
```

## Health Check

```bash
# No auth required
curl https://your-domain:8443/api/v1/health
```

## Using API Keys

```bash
# Generate API key (admin only)
curl -X POST /api/v1/admin/apikeys \
  -H "Authorization: Bearer $TOKEN"

# Use API key instead of JWT
curl /api/v1/apps \
  -H "X-API-Key: dm_your_api_key_here"
```
