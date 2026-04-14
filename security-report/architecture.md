# DeployMonster Architecture Reconnaissance Report

## Executive Summary

**DeployMonster** is a self-hosted Platform-as-a-Service (PaaS) application written primarily in Go (backend) with a React/TypeScript frontend. It provides containerized application deployment, database management, domain handling, and team collaboration features. The application follows a modular monolith architecture with an event-driven design.

---

## 1. Technology Stack Detection

### Backend (Go)
- **Language**: Go 1.26.1
- **Total Go Files**: 593 files
- **Total Lines of Code**: ~66,000 LOC (backend only)
- **Web Framework**: Standard library `net/http` with custom router
- **Key Dependencies**:
  - `github.com/docker/docker` v28.5.2 - Docker SDK client
  - `github.com/golang-jwt/jwt/v5` v5.3.1 - JWT authentication
  - `github.com/gorilla/websocket` v1.5.3 - WebSocket support
  - `github.com/jackc/pgx/v5` v5.9.1 - PostgreSQL driver
  - `github.com/mattn/go-isatty` v0.0.21 - Terminal detection
  - `go.etcd.io/bbolt` v1.4.3 - Embedded key-value store
  - `golang.org/x/crypto` v0.50.0 - Cryptographic operations
  - `gopkg.in/yaml.v3` v3.0.1 - YAML configuration
  - `modernc.org/sqlite` v1.48.2 - Pure Go SQLite

### Frontend (React/TypeScript)
- **Framework**: React 19.2.5 with TypeScript 5.9.3
- **Build Tool**: Vite 8.0.5
- **Router**: React Router 7.13.2
- **State Management**: Zustand 5.0.12
- **Styling**: Tailwind CSS 4.2.2
- **UI Components**: Custom components with class-variance-authority
- **Testing**: Vitest 3.2.1, React Testing Library
- **TypeScript Files**: 134 files (84 .tsx, 50 .ts)

### Databases
1. **SQLite** (default) - Primary application database with WAL mode
2. **PostgreSQL** (enterprise) - Alternative production database
3. **BBolt** - Embedded key-value store for sessions, rate limiting, caching, metrics

### Build & Deployment
- **Build Tool**: GoReleaser with goreleaser.yml
- **Container**: Multi-stage Dockerfile (scratch-based runtime)
- **Package Manager**: pnpm (frontend)
- **CI/CD**: GitHub Actions (`.github/workflows/`)

---

## 2. Application Type Classification

**Primary Classification**: Self-hosted PaaS (Platform-as-a-Service) Web Application

**Architecture Pattern**: Modular Monolith with Event-Driven Components

**Application Characteristics**:
- **Web Application**: Yes - Single Page Application (SPA) with React frontend
- **REST API**: Yes - Full REST API at `/api/v1/*`
- **GraphQL API**: No
- **CLI Tool**: Yes - Single binary with embedded UI
- **Microservice**: No - Modular monolith design
- **WebSocket Support**: Yes - Real-time features for deployment progress, logs, terminal
- **Server-Side Events (SSE)**: Yes - For log streaming and event broadcasting

---

## 3. Entry Points Mapping

### HTTP Routes (REST API)

**Public Routes** (no authentication):
- `GET /health` - Basic health check
- `GET /api/v1/health` - Detailed health status
- `GET /readyz` - Readiness probe for load balancers
- `GET /api/v1/openapi.json` - OpenAPI specification
- `POST /api/v1/auth/login` - Authentication (rate-limited)
- `POST /api/v1/auth/register` - User registration (rate-limited)
- `POST /api/v1/auth/refresh` - Token refresh (rate-limited)
- `POST /api/v1/auth/logout` - Logout
- `GET /api/v1/environments/presets` - Environment presets
- `GET /api/v1/marketplace` - Public marketplace listings
- `GET /api/v1/marketplace/{slug}` - Marketplace template details
- `GET /api/v1/databases/engines` - Database engine types
- `POST /hooks/v1/{webhookID}` - Inbound webhooks (signature-verified)
- `POST /api/v1/webhooks/stripe` - Stripe webhook (when billing enabled)

**Protected Routes** (JWT/API Key required):
Key route groups include:
- **Apps**: `/api/v1/apps/*` - CRUD operations, deployment, scaling, logs
- **Projects**: `/api/v1/projects/*` - Project management
- **Domains**: `/api/v1/domains/*` - Domain management and verification
- **Databases**: `/api/v1/databases/*` - Managed database operations
- **Servers**: `/api/v1/servers/*` - VPS provisioning and management
- **Secrets**: `/api/v1/secrets/*` - Secret management
- **Backups**: `/api/v1/backups/*` - Backup operations
- **Team**: `/api/v1/team/*` - Team and invitation management
- **Billing**: `/api/v1/billing/*` - Usage and billing (when enabled)
- **Admin**: `/api/v1/admin/*` - Super admin only endpoints

**Admin-Only Routes**:
- `GET /api/v1/admin/system` - System information
- `PATCH /api/v1/admin/settings` - Update system settings
- `GET /api/v1/admin/tenants` - List all tenants
- `POST /api/v1/apps/{id}/transfer` - Cross-tenant app transfer

### WebSocket Endpoints
- `GET /api/v1/topology/deploy/{projectId}/progress` - Deployment progress (WebSocket)
- `GET /api/v1/events/stream` - Event streaming (SSE)
- `GET /api/v1/apps/{id}/logs/stream` - Log streaming (SSE)
- `GET /api/v1/apps/{id}/terminal` - Interactive terminal (WebSocket)

### CLI Entry Points
- `cmd/deploymonster/main.go` - Main application entry
- `cmd/openapi-gen/main.go` - OpenAPI documentation generator

---

## 4. Data Flow Map

### Sources (User Input)
1. **HTTP Request Parameters**: Path values (`r.PathValue()`), Query parameters
2. **Request Body**: JSON payloads (max 10MB global, 1MB for webhooks)
3. **Headers**: Authorization, X-API-Key, X-CSRF-Token, X-Request-ID
4. **Cookies**: `dm_access` (JWT), `dm_refresh` (refresh token), `__Host-dm_csrf` (CSRF)
5. **WebSocket Messages**: Real-time commands and data
6. **Webhooks**: External service callbacks (GitHub, Stripe, etc.)
7. **File Uploads**: Docker Compose files, certificates, backups

### Processing
1. **Middleware Chain** (in order):
   - Request ID generation
   - Graceful shutdown handling
   - Global rate limiting (120 req/min default)
   - Security headers
   - API metrics collection
   - API version header
   - Body size limiting (10MB)
   - Request timeout (30s)
   - Panic recovery
   - Request logging
   - CORS handling
   - CSRF protection (cookie-based double-submit)
   - Idempotency key handling
   - Audit logging

2. **Authentication**:
   - JWT Bearer token validation (Authorization header)
   - Cookie-based JWT validation
   - API Key validation (X-API-Header with `dm_` prefix)

3. **Authorization**:
   - RBAC with roles: super_admin, owner, admin, developer, operator, viewer, billing
   - Tenant-scoped access control
   - Admin-only middleware for platform endpoints

4. **Validation**:
   - JSON schema validation
   - Input sanitization
   - Path traversal prevention (volume mounts)
   - SQL injection prevention (parameterized queries)

### Sinks
1. **Database Writes**:
   - SQLite/PostgreSQL via `core.Store` interface
   - BBolt for key-value operations
   - Encrypted fields for secrets

2. **Docker Operations**:
   - Container creation/start/stop/remove
   - Image pull/build
   - Network and volume management
   - Exec commands in containers

3. **File System**:
   - Build directories (temporary)
   - Backup storage
   - Certificate files
   - SQLite database files

4. **External Services**:
   - Git providers (GitHub, GitLab, etc.)
   - VPS providers (Hetzner, DigitalOcean, etc.)
   - DNS providers (Cloudflare)
   - Notification channels (SMTP, Slack, Discord, Telegram)
   - Stripe (billing)

5. **HTTP Responses**:
   - JSON API responses
   - WebSocket messages
   - SSE events
   - Static asset serving (embedded React UI)

---

## 5. Trust Boundaries

### Authentication Boundaries
1. **Public/Unauthenticated**:
   - Health endpoints, login, register, refresh
   - Rate limited by IP

2. **Authenticated**:
   - All `/api/v1/*` routes except auth
   - Requires valid JWT or API Key

3. **Admin-Only**:
   - `/api/v1/admin/*` routes
   - Cross-tenant operations
   - Requires `role_super_admin`

### Rate Limiting
- **Global Rate Limit**: 120 requests/minute per IP (configurable)
- **Auth Rate Limiting**: 5 requests/minute for login/register/refresh
- **Tenant Rate Limiting**: 100 requests/minute per tenant
- **WebSocket Rate Limiting**: Applied per connection

### Input Validation
- **Body Size Limits**: 10MB default, 1MB for webhooks
- **CSRF Protection**: Double-submit cookie pattern for cookie-based auth
- **Path Traversal Prevention**: `filepath.Clean()` and `..` detection
- **Docker Socket Protection**: Blocked unless explicitly allowed

### CORS Configuration
- Configurable via `cors_origins` setting
- Supports wildcard (`*`) or specific origins
- Credentials only allowed with explicit origin match
- Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
- Headers: Content-Type, Authorization, X-API-Key, X-Request-ID, X-CSRF-Token

### Security Headers (via middleware)
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 0`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Content-Security-Policy` (comprehensive policy)
- `Strict-Transport-Security` (HSTS, TLS only)
- `Cache-Control: no-store` for API routes

---

## 6. External Integrations

### Container Runtime
- **Docker**: Primary container runtime via Docker SDK
  - Socket: `unix:///var/run/docker.sock` (configurable)
  - Features: Build, run, stop, remove containers
  - Security: `no-new-privileges`, capability dropping, resource limits

### Databases
1. **SQLite** (default):
   - File: `deploymonster.db`
   - WAL mode enabled
   - Embedded migrations
   - Single writer, multiple readers

2. **PostgreSQL** (enterprise):
   - Connection via `pgx` driver
   - Connection pooling (25 max open, 5 max idle)
   - Same migration system as SQLite

3. **BBolt**: Embedded key-value store
   - Sessions, rate limiting, caching
   - TTL support for keys

### Git Providers
- GitHub, GitLab, Bitbucket, Gitea, Gogs, Azure DevOps, CodeCommit
- OAuth and Personal Token authentication
- Webhook management

### VPS Providers
- Hetzner, DigitalOcean, Vultr, AWS, GCP, Azure
- Server provisioning and management
- SSH key management

### DNS Providers
- Cloudflare (primary)
- Route53
- Manual DNS

### Notification Channels
- SMTP (email)
- Slack webhooks
- Discord webhooks
- Telegram bots

### Payment Processing
- Stripe (optional)
- Webhook signature verification

---

## 7. Authentication Architecture

### JWT Implementation
- **Library**: `github.com/golang-jwt/jwt/v5`
- **Access Token**: 15 minutes expiration
- **Refresh Token**: 7 days expiration
- **Signing Method**: HS256 (HMAC-SHA256)
- **Claims**: UserID, TenantID, RoleID, Email

### Token Storage
- **Access Token**: `dm_access` cookie (httpOnly, Secure flag based on TLS)
- **Refresh Token**: `dm_refresh` cookie (httpOnly, Secure)
- **CSRF Token**: `__Host-dm_csrf` cookie (NOT httpOnly, readable by JS)

### Key Rotation
- Supports graceful key rotation with `previous_secret_keys`
- Grace period: 1 hour for old keys
- Automatic purging of expired previous keys

### API Key Authentication
- **Format**: `dm_` prefix followed by random string
- **Storage**: SHA-256 hash in database
- **Prefix Lookup**: First 8 characters for identification
- **Scopes**: JSON array of allowed permissions

### Password Security
- **Hashing**: bcrypt with default cost
- **Strength Validation**: Configurable minimum length
- **First Run Setup**: Auto-generates admin password if no users exist

### Session Management
- Token-based (stateless)
- Refresh token rotation on refresh
- Logout invalidates tokens client-side

---

## 8. File Structure Analysis

### Configuration Files
- `monster.yaml` / `monster.example.yaml` - Primary configuration
- `.env.example` - Environment variable template
- `go.mod` / `go.sum` - Go dependencies
- `web/package.json` - Node.js dependencies

### Sensitive Paths
**Protected by Authentication**:
- `/admin` - Admin dashboard (SPA route)
- `/api/v1/admin/*` - Admin API endpoints
- `/metrics` - Prometheus metrics (auth-protected)
- `/debug/pprof/*` - Go profiling (opt-in, auth-protected)

**Public Health Endpoints**:
- `/health` - Basic health check
- `/readyz` - Kubernetes readiness probe
- `/health/detailed` - Detailed health status

### Deployment Files
- `Dockerfile` - Production multi-stage build
- `deployments/Dockerfile` - Development build
- `docker-compose.yml` - Production deployment
- `docker-compose.prod.yml` - Production with Postgres
- `docker-compose.postgres.yml` - Postgres variant

### Key Directories
- `/internal/api/` - HTTP handlers, middleware, WebSocket
- `/internal/auth/` - JWT, password hashing, API keys
- `/internal/db/` - Database abstraction, migrations, models
- `/internal/core/` - Core interfaces, events, services
- `/internal/deploy/` - Docker container management
- `/internal/build/` - Build pipeline, git operations
- `/internal/secrets/` - Encryption vault
- `/web/src/` - React frontend source

---

## 9. Detected Security Controls

### Existing Security Measures

1. **Authentication & Authorization**:
   - JWT-based authentication with short-lived tokens
   - API Key support for service accounts
   - RBAC with 7 built-in roles
   - Super admin isolation for platform operations

2. **Input Validation**:
   - JSON body size limits (10MB default)
   - Parameterized SQL queries
   - Path traversal prevention in volume mounts
   - Docker socket access control

3. **Rate Limiting**:
   - Global per-IP rate limiting
   - Auth endpoint-specific limits
   - Tenant-level rate limiting
   - BBolt-backed rate limit storage

4. **CSRF Protection**:
   - Double-submit cookie pattern
   - Exempt for API Key and Bearer token auth
   - Secure cookie flags

5. **Security Headers**:
   - Comprehensive CSP policy
   - HSTS (TLS only)
   - X-Frame-Options: DENY
   - X-Content-Type-Options: nosniff

6. **Docker Security**:
   - `no-new-privileges` flag
   - Capability dropping (ALL, then selective add)
   - Resource limits (CPU, memory)
   - Log rotation configuration

7. **Encryption**:
   - AES-256-GCM for secret encryption
   - bcrypt for password hashing
   - SHA-256 for API key hashing
   - Random salt generation

8. **Audit & Logging**:
   - Structured JSON logging
   - Audit log middleware
   - Request ID tracking
   - Panic recovery with logging

9. **Build Security**:
   - Scratch-based production image
   - Non-root user (65534:65534)
   - No shell in production image
   - Minimal attack surface

10. **CORS Security**:
    - Origin validation
    - Credentials only with explicit origins
    - Proper header exposure

---

## 10. Language Detection Summary

| Language | Files | Approximate LOC | Percentage |
|----------|-------|-----------------|------------|
| Go | 593 | ~66,000 | 88% |
| TypeScript (.tsx) | 84 | ~8,400 | 11% |
| TypeScript (.ts) | 50 | ~1,500 | 2% |
| CSS | 9 | ~500 | <1% |
| **Total** | **736** | **~76,000** | **100%** |

### Language-Specific Skills to Activate

1. **Go Deep Scanner** (Primary):
   - JWT validation logic
   - SQL injection prevention
   - Docker command execution
   - File path handling
   - Cryptographic implementations
   - Race condition detection

2. **TypeScript/React Scanner** (Secondary):
   - Frontend API client security
   - XSS prevention in JSX
   - CSRF token handling
   - Authentication flow

3. **Docker Security Scanner**:
   - Dockerfile best practices
   - Container escape prevention
   - Volume mount security

---

## Recommendations for Security Assessment

### High-Priority Areas
1. **Docker Integration** (`internal/deploy/docker.go`, `internal/build/builder.go`)
2. **Authentication Flow** (`internal/auth/jwt.go`, `internal/api/middleware/middleware.go`)
3. **Database Layer** (`internal/db/*.go`)
4. **Secret Management** (`internal/secrets/vault.go`)
5. **WebSocket Handlers** (`internal/api/ws/*.go`)

### Medium-Priority Areas
1. **API Handlers** (`internal/api/handlers/*.go`)
2. **Git Operations** (`internal/build/builder.go`)
3. **External Service Integrations** (`internal/gitsources/`, `internal/vps/`)

### Low-Priority Areas
1. **Frontend** (`web/src/`)
2. **Configuration** (`internal/core/config.go`)
3. **Event System** (`internal/core/events.go`)

---

**Report Generated**: 2026-04-14
**Analyzed By**: Claude Code Security Reconnaissance Skill
**Codebase Version**: 0.5.2-SNAPSHOT (f3b75de)
