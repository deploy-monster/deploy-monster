# Architecture Map - DeployMonster

## Backend: Go 1.26+ Modular Monolith

### Core Modules (20)
api, auth, backup, billing, build, database, db, deploy, discovery, dns, enterprise, gitsources, ingress, marketplace, mcp, notifications, resource, secrets, swarm, topology, vps, webhooks

### Key Interfaces (internal/core/)
- Store (12 sub-interfaces)
- ContainerRuntime (Docker operations)
- SecretResolver, NotificationSender, DNSProvider, BackupStorage, VPSProvisioner, GitProvider

### API Layer
- Go 1.22+ http.ServeMux
- Middleware chain: RequestID → RateLimit → BodyLimit(10MB) → Timeout(30s) → Recovery → CORS → CSRF → AuditLog
- Auth: JWT (HS256), API keys, TOTP MFA

### Database
- SQLite (WAL mode) + BBolt KV + PostgreSQL (planned)

## Frontend: React 19 + Vite 8 + TypeScript

### State: Zustand 5 (4 stores: auth, theme, toast, topology)
### API: Cookie-based auth with CSRF protection

### Security-Critical Files
- `web/src/stores/auth.ts` - JWT decoding WITHOUT verification (CRITICAL)
- `web/src/pages/Admin.tsx` - Missing RBAC (HIGH)
- `web/src/pages/Onboarding.tsx:127` - localStorage state
