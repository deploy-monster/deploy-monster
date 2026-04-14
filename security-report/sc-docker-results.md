# Docker and Infrastructure Security Scan Report

**Scan Date:** 2026-04-14  
**Target:** DeployMonster PaaS Codebase (D:\CODEBOX\PROJECTS\DeployMonster_GO)  
**Scan Type:** Comprehensive Docker & Infrastructure Security Analysis

---

## Executive Summary

| Category | Findings | Severity Distribution |
|----------|----------|---------------------|
| **Positive Security Controls** | 12 | N/A (Good Practices) |
| **Medium Severity** | 3 | Requires attention |
| **Low Severity** | 2 | Recommend improvements |
| **Informational** | 4 | Documentation notes |

**Overall Assessment:** The codebase demonstrates strong Docker security practices with multi-stage builds, non-root users, minimal base images (distroless/scratch), and capability dropping. Key concerns exist around privileged marketplace containers and Docker socket mounting from untrusted compose templates.

---

## 1. Positive Security Controls (Good Practices)

### DKR-GOOD-001: Distroless/Scratch Production Image
**Location:** `Dockerfile` (root directory)  
**Status:** SECURE

The production Dockerfile uses a minimal scratch-based image with only CA certificates and tzdata:

```dockerfile
FROM alpine:3.21 AS rootfs
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /rootfs/var/lib/deploymonster \
    && chown -R 65534:65534 /rootfs/var/lib/deploymonster

FROM scratch
COPY --from=rootfs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=rootfs /usr/share/zoneinfo /usr/share/zoneinfo
USER 65534:65534
```

**Security Benefits:**
- No shell, no package manager, no curl
- Smallest possible attack surface
- No unused binaries or libraries

---

### DKR-GOOD-002: Non-Root User in Development Dockerfile
**Location:** `deployments/Dockerfile`  
**Status:** SECURE

```dockerfile
RUN addgroup -S monster \
    && adduser -S monster -G monster
# ...
USER monster
```

The development image runs as dedicated `monster` user, preventing container escape via root exploits.

---

### DKR-GOOD-003: no-new-privileges Security Option
**Location:** `internal/deploy/docker.go:103`  
**Status:** SECURE

```go
hostCfg := &container.HostConfig{
    SecurityOpt: []string{"no-new-privileges"},
    // ...
}
```

All containers are started with `no-new-privileges` preventing privilege escalation via setuid binaries.

---

### DKR-GOOD-004: Capability Dropping with Selective Add
**Location:** `internal/deploy/docker.go:117-123`  
**Status:** SECURE

```go
hostCfg.CapDrop = []string{"ALL"}
hostCfg.CapAdd = []string{
    "CHOWN", "SETUID", "SETGID",
    "NET_BIND_SERVICE",
    "DAC_OVERRIDE",
}
```

Follows principle of least privilege - drops ALL capabilities then adds back only what's needed.

---

### DKR-GOOD-005: Docker Socket Mount Validation
**Location:** `internal/core/interfaces.go:59-114`  
**Status:** SECURE

```go
var dangerousPaths = []string{
    "/var/run/docker.sock",
    "/run/docker.sock",
    "/var/run/docker",
}

// Block Docker socket mounts unless explicitly allowed
if !o.AllowDockerSocket {
    for _, dangerous := range dangerousPaths {
        if normalizedPath == dangerous {
            return fmt.Errorf("volume host path %q is blocked", hostPath)
        }
    }
}
```

Explicit `AllowDockerSocket` flag required for mounting Docker socket, with path traversal protection.

---

### DKR-GOOD-006: Path Traversal Protection in Volume Paths
**Location:** `internal/core/interfaces.go:69-114`  
**Status:** SECURE

```go
// Pre-cleaning check for raw traversal attempts
if strings.Contains(hostPath, "..") {
    return fmt.Errorf("volume host path %q contains path traversal", hostPath)
}

// Post-cleaning check
if strings.Contains(cleaned, "..") {
    return fmt.Errorf("volume host path %q contains path traversal after cleaning", hostPath)
}

// Must be absolute
if !filepath.IsAbs(cleaned) {
    return fmt.Errorf("volume host path %q must be absolute", hostPath)
}
```

Multi-layer path traversal validation prevents volume mount escapes.

---

### DKR-GOOD-007: Production Docker Compose Hardening
**Location:** `docker-compose.prod.yml`  
**Status:** SECURE

```yaml
read_only: true
tmpfs:
  - /tmp:size=100M
security_opt:
  - no-new-privileges:true
deploy:
  resources:
    limits:
      memory: 512M
      cpus: "1.0"
```

Production compose includes:
- Read-only root filesystem
- Limited tmpfs for /tmp
- Resource limits
- Security options

---

### DKR-GOOD-008: Systemd Service Security Hardening
**Location:** `deployments/deploymonster.service`  
**Status:** SECURE

```ini
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictNamespaces=true
LockPersonality=true
```

Comprehensive systemd hardening with sandboxing directives.

---

### DKR-GOOD-009: Log Rotation Configuration
**Location:** `internal/deploy/docker.go:104-110`  
**Status:** SECURE

```go
LogConfig: container.LogConfig{
    Type: "json-file",
    Config: map[string]string{
        "max-size": "50m",
        "max-file": "5",
    },
},
```

Prevents log exhaustion attacks with size limits and rotation.

---

### DKR-GOOD-010: .dockerignore Excludes Sensitive Files
**Location:** `.dockerignore`  
**Status:** SECURE

```
.env
.env.*
monster.yaml
.credentials
*.db
*.db-wal
*.db-shm
*.bolt
```

Excludes secrets, databases, and environment files from build context.

---

### DKR-GOOD-011: PostgreSQL Security in Compose
**Location:** `docker-compose.postgres.yml`  
**Status:** SECURE

```yaml
# SECURITY FIX: Remove host port binding - use internal Docker network only
# ports:
#   - "5432:5432"
```

PostgreSQL container is NOT exposed on host port - only accessible via internal Docker network.

---

### DKR-GOOD-012: Docker Socket Read-Only Mount
**Location:** Multiple docker-compose files  
**Status:** SECURE

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

Docker socket is mounted read-only (`:ro`) where required.

---

## 2. Medium Severity Findings

### DKR-001: Privileged Mode Enabled for Marketplace Templates
**Location:** `internal/marketplace/builtins_100.go:1305`  
**Severity:** MEDIUM  
**CWE:** CWE-250 (Execution with Unnecessary Privileges)

**Finding:**
The Home Assistant marketplace template runs with `privileged: true`:

```yaml
homeassistant:
  image: ghcr.io/home-assistant/home-assistant:stable
  privileged: true
```

**Risk:**
Privileged containers have full access to host devices, can access kernel features, and bypass most security controls. If compromised, complete host takeover is possible.

**Code Handling:**
The container runtime code in `internal/deploy/docker.go:113-116` does handle privileged mode:

```go
if opts.Privileged {
    hostCfg.Privileged = true
    hostCfg.SecurityOpt = nil // Privileged mode overrides security opts
}
```

When privileged is enabled, all security options including `no-new-privileges`, capability dropping, and other hardening are **removed**.

**Remediation:**
1. Add strict validation and approval workflow for privileged containers
2. Document which marketplace apps require privileged mode and why
3. Consider requiring explicit admin confirmation before deploying privileged marketplace apps
4. Add audit logging for privileged container deployments

---

### DKR-002: Docker Socket Mount in Marketplace Templates
**Location:** Multiple builtin files  
**Severity:** MEDIUM  
**Files:**
- `internal/marketplace/builtins_100.go:804` (Portainer)
- `internal/marketplace/builtins_100.go:1247` (Woodpecker Agent)
- `internal/marketplace/builtins_extended.go:408` (Traefik)
- `internal/marketplace/builtins_extra.go:119` (Portainer)

**Finding:**
Multiple marketplace templates mount Docker socket without read-only flag:

```yaml
portainer:
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock
```

**Risk:**
Access to Docker socket equals root on host. These marketplace apps can create, delete, and modify containers on the host system.

**Current Mitigation:**
The `AllowDockerSocket` flag exists (`internal/core/interfaces.go:55`) but the builtin templates are implicitly trusted. No explicit user confirmation is required when deploying these templates.

**Remediation:**
1. Add `:ro` (read-only) to Docker socket mounts where possible
2. Display prominent security warning when deploying socket-mounting templates
3. Require explicit user acknowledgment for templates requiring Docker socket access
4. Consider running these in isolated Docker contexts or rootless Docker

---

### DKR-003: smoke-docker.sh Uses Privileged Mode
**Location:** `scripts/smoke-docker.sh:33`  
**Severity:** MEDIUM

**Finding:**
```bash
docker run --rm \
  --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
```

**Risk:**
Test script runs privileged containers. While acceptable for CI/testing, this could be accidentally copied for production use.

**Remediation:**
Add prominent comment warning that `--privileged` is for testing only:
```bash
# WARNING: --privileged is required for Docker-in-Docker in tests only
# DO NOT use in production deployments
```

---

## 3. Low Severity Findings

### DKR-004: MONSTER_SECRET in Environment Variables
**Location:** `docker-compose.yml:21`, `docker-compose.prod.yml:19`  
**Severity:** LOW

**Finding:**
```yaml
environment:
  - MONSTER_SECRET=${MONSTER_SECRET:-}
```

**Risk:**
Secrets passed via environment variables may be visible in:
- `docker inspect` output
- Process listing (`ps e`)
- Container logs if application dumps environment

**Remediation:**
1. Use Docker secrets (swarm mode) or bind-mounted secret files:
```yaml
secrets:
  - monster_secret
```

2. Or mount as file and read from path:
```yaml
volumes:
  - ./secrets/monster_secret:/run/secrets/monster_secret:ro
```

---

### DKR-005: Missing Image Digest Pinning
**Location:** `deployments/Dockerfile`, `docker-compose*.yml`  
**Severity:** LOW

**Finding:**
Images use floating tags which can be mutated:
```dockerfile
FROM alpine:3.21
FROM node:22-alpine
FROM golang:1.26-alpine
```

**Risk:**
Supply chain attack via compromised registry or tag mutation.

**Remediation:**
Pin to specific digests:
```dockerfile
FROM alpine:3.21@sha256:sha256:abc123...
```

Or use Dependabot/Renovate to track updates while maintaining pins.

---

## 4. Informational Findings

### DKR-INFO-001: PostgreSQL Password in Connection String
**Location:** `docker-compose.postgres.yml:35`  
**Severity:** INFO

**Finding:**
```yaml
MONSTER_DATABASE_URL=postgres://...:${POSTGRES_PASSWORD}@postgres:5432/...?sslmode=disable
```

The connection string includes password and disables SSL (`sslmode=disable`). This is acceptable for internal Docker network communication but should be documented.

**Note:** The comment at line 4-5 correctly warns users to set `POSTGRES_PASSWORD` securely.

---

### DKR-INFO-002: Docker Compose Version Syntax
**Location:** All docker-compose files  
**Severity:** INFO

All compose files use modern `version: "3.8"` syntax which supports security features like `read_only` and `security_opt`. Good practice.

---

### DKR-INFO-003: Health Check Configuration
**Location:** `docker-compose.yml:22-27`, `docker-compose.prod.yml:27-32`  
**Severity:** INFO

Health checks are properly configured with reasonable timeouts:
```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "-k", "https://localhost:8443/api/v1/health"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 10s
```

---

### DKR-INFO-004: Capability Set Review
**Location:** `internal/deploy/docker.go:119-123`  
**Severity:** INFO

Current capability set:
```go
hostCfg.CapAdd = []string{
    "CHOWN", "SETUID", "SETGID",
    "NET_BIND_SERVICE",
    "DAC_OVERRIDE",
}
```

**Review Notes:**
- `NET_BIND_SERVICE`: Required for binding ports <1024
- `CHOWN/SETUID/SETGID`: File ownership operations
- `DAC_OVERRIDE`: Bypass file permission checks

Consider if all are necessary for all workloads or if some can be opt-in per-app.

---

## 5. Security Controls Matrix

| Control | Production Dockerfile | Dev Dockerfile | docker-compose.prod.yml | Go Runtime | Status |
|---------|----------------------|----------------|------------------------|------------|--------|
| Non-root user | UID 65534 (nobody) | `monster` user | N/A | N/A | PASS |
| Multi-stage build | Yes (2 stages) | Yes (3 stages) | N/A | N/A | PASS |
| Minimal base | Scratch | Alpine | N/A | N/A | PASS |
| no-new-privileges | N/A | N/A | Yes | Yes | PASS |
| Read-only rootfs | N/A | N/A | Yes | Via API | PASS |
| tmpfs for /tmp | N/A | N/A | Yes | Via API | PASS |
| Capability dropping | N/A | N/A | N/A | Yes | PASS |
| Resource limits | N/A | N/A | Yes | Yes | PASS |
| Docker socket validation | N/A | N/A | N/A | Yes | PASS |
| Log rotation | N/A | N/A | N/A | Yes | PASS |
| Secrets in env vars | No | No | Partial | No | PARTIAL |

---

## 6. Recommendations

### Immediate Actions (High Priority)
1. **DKR-001**: Add approval workflow for privileged marketplace containers
2. **DKR-002**: Add security warnings for Docker socket mounting templates
3. **DKR-003**: Document privileged flag in smoke-docker.sh as test-only

### Short-term Improvements (Medium Priority)
4. **DKR-004**: Migrate MONSTER_SECRET to Docker secrets or mounted file
5. **DKR-005**: Pin image digests in Dockerfiles
6. Add container security scanning (Trivy/Grype) to CI pipeline

### Long-term Hardening (Lower Priority)
7. Implement Podman/rootless Docker support
8. Add runtime security monitoring (Falco)
9. Consider gVisor or Kata Containers for tenant isolation
10. Implement image signing and verification (Cosign)

---

## 7. References

- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [CIS Docker Benchmark v1.5.0](https://www.cisecurity.org/benchmark/docker)
- [NIST SP 800-190: Application Container Security Guide](https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-190.pdf)
- Docker Security Documentation: https://docs.docker.com/engine/security/

---

## 8. Appendix: File Locations Summary

| File | Purpose | Security Notes |
|------|---------|----------------|
| `Dockerfile` | Production build (scratch-based) | Excellent - distroless |
| `deployments/Dockerfile` | Dev build (Alpine-based) | Good - non-root user |
| `docker-compose.yml` | Base compose config | Basic, needs hardening |
| `docker-compose.prod.yml` | Production compose | Well hardened |
| `docker-compose.postgres.yml` | PostgreSQL addon | Good - no host port binding |
| `deployments/docker-compose.dev.yaml` | Dev compose | Has socket mount |
| `deployments/deploymonster.service` | Systemd unit | Excellent hardening |
| `internal/deploy/docker.go` | Container runtime | Good - secopts, capabilities |
| `internal/core/interfaces.go` | Volume validation | Good - path traversal protection |
| `internal/marketplace/builtins*.go` | App templates | Needs privileged/socket review |
| `scripts/smoke-docker.sh` | Test script | Uses --privileged |
| `.dockerignore` | Build context | Good exclusions |

---

*Report generated by Claude Code Security Scanner v1.0*
