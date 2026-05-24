# sc-docker Results

## Summary
Docker and container build security scan.

## Findings

### DOCKER-001: Build Pipeline Isolation Depends On Docker Daemon Policy
- **Severity:** Medium
- **Confidence:** 70
- **File:** `internal/build/builder.go`
- **Description:** Docker builds are an intentional PaaS capability and run through the host/agent Docker runtime. Build args and command construction are hardened, but build isolation still depends on Docker daemon configuration, worker trust boundaries, BuildKit settings, and host-level controls.
- **Remediation:** Run builds on isolated workers, keep Docker daemon access restricted, prefer rootless/buildkit sandboxing where feasible, and document required daemon hardening for production clusters.

## Resolved / Revalidated Items

### DOCKER-002: Docker Client v28.5.2 Known Vulnerabilities
- **Previous Severity:** High
- **Status:** RESOLVED / NOT CURRENT
- **File:** `go.mod`
- **Notes:** The legacy `github.com/docker/docker@v28.5.2+incompatible` module is no longer required or imported. The project currently uses split Moby modules (`github.com/moby/moby/api`, `github.com/moby/moby/client`), and `govulncheck ./...` reports 0 called vulnerabilities.

### DOCKER-003: Generated Dockerfiles Run As Root
- **Previous Severity:** Medium
- **Status:** MOSTLY RESOLVED / TEMPLATE-SCOPED
- **File:** `internal/build/dockerfiles.go`
- **Notes:** Generated app Dockerfiles set non-root users for Node, Next.js, Nuxt, Go, Rust, Python, PHP, Java, .NET, and Ruby templates. Static/Vite templates use `nginx:alpine`, which provides an nginx user, but the generated config still listens on port 80 and should be rechecked if nginx runtime assumptions change. User-supplied Dockerfiles remain outside this template guarantee.

## Positive Security Patterns Observed
- Docker build args are validated against injection.
- Image tags are validated against format patterns.
- Build timeout is enforced.
- `--force-rm` is used to clean up intermediate containers.
- Custom Dockerfile paths are contained inside the build context before `docker build -f`.
