# Phase 2: Infrastructure, Docker, and CI/CD Vulnerabilities

## D1 — Docker Socket Mounted Without Read-Only Flag (Host Root Escape Risk)

**File:** `docker-compose.yml:16`

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

**Why it's a vulnerability:**

Mounting the Docker socket without `:ro` (read-only) grants the container full write access to the Docker daemon on the host. A compromised DeployMonster container can use this socket to create privileged containers, mount host directories, or escape entirely out of the container boundary. The socket gives effective root-level control over the entire host.

Contrast with `docker-compose.prod.yml:16` which correctly uses `:ro`.

**CWE:** CWE-284 (Improper Access Control) / CWE-668 (Exposure of Resource to Wrong Sphere)

---

## D2 — Docker Socket Mounted Without Read-Only in Dev Compose

**File:** `deployments/docker-compose.dev.yaml:12`

```yaml
- /var/run/docker.sock:/var/run/docker.sock
```

**Why it's a vulnerability:**

Same issue as D1, but in the development compose file. A misconfiguration in dev can migrate to production workflows. Should use `:ro` variant for consistency.

**CWE:** CWE-284 / CWE-668

---

## D3 — Missing USER Directive in Root Dockerfile

**File:** `Dockerfile:1-42` (root `Dockerfile`, consumed by goreleaser)

The root `Dockerfile` does NOT have a `USER` directive before `ENTRYPOINT`. The multi-stage build ends with `USER 65534:65534` in the `deployments/Dockerfile` (stage 3), but the release Dockerfile at the repository root does not set a non-root user. The `ENTRYPOINT ["/deploymonster"]` runs as root.

**Why it's a vulnerability:**

Running as root inside a container means any container escape or privilege escalation immediately grants root on the host (unless the Docker daemon is configured with user namespace remapping, which is not assumed). An attacker who compromises the process gains root capabilities without needing a container breakout.

**CWE:** CWE-250 (Execution with Unnecessary Privileges) / CWE-284

---

## D4 — No Healthcheck in Dev Docker Compose

**File:** `deployments/docker-compose.dev.yaml:1-20`

```yaml
services:
  deploymonster:
    build:
      context: ..
      dockerfile: deployments/Dockerfile
    ports:
      - "8443:8443"
      - "80:80"
      - "443:443"
    volumes:
      - dm-data:/var/lib/deploymonster
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - MONSTER_HOST=0.0.0.0
      - MONSTER_PORT=8443
      - MONSTER_LOG_LEVEL=debug
    restart: unless-stopped
```

**Why it's a vulnerability:**

No `healthcheck` block is defined. Docker cannot determine if the container is healthy and will not restart a crashed or hung DeployMonster instance automatically. Compare with `docker-compose.prod.yml:27-32` which has a proper healthcheck defined.

**CWE:** CWE-400 (Uncontrolled Resource Consumption) via missing failure detection

---

## D5 — No Healthcheck in Base Docker Compose

**File:** `docker-compose.yml:1-31`

The base `docker-compose.yml` also lacks a healthcheck on the `deploymonster` service.

**CWE:** CWE-400

---

## D6 — Hardcoded PostgreSQL Credentials in Compose

**File:** `docker-compose.postgres.yml:11-14`

```yaml
environment:
  POSTGRES_USER: deploymonster
  POSTGRES_PASSWORD: deploymonster
  POSTGRES_DB: deploymonster
```

**Why it's a vulnerability:**

Credentials are set in plaintext in a compose file. While this is a default/development configuration, the same file structure is used as a template. Credentials in compose files can leak into CI logs, artifact metadata, or version control history. The password should be sourced from a secret management mechanism or environment variable with a generated default.

**CWE:** CWE-798 (Use of Hard-coded Credentials)

---

## D7 — Default Registration Mode Set to "open"

**File:** `monster.example.yaml:53`

```yaml
registration:
  mode: open                    # open, invite_only, approval, disabled
```

**Why it's a vulnerability:**

`open` registration allows anyone to create a tenant account without invitation or approval. This could lead to unauthorized platform usage, resource exhaustion, or abuse. The more secure default is `invite_only` or `approval`. While this is an example file, it serves as the template for new deployments.

**CWE:** CWE-284 (Improper Access Control)

---

## D8 — GITHUB_TOKEN Used for Docker Login in Release Workflow

**File:** `.github/workflows/release.yml:46-51`

```yaml
- name: Log in to GHCR
  uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
```

**Why it's a vulnerability:**

`secrets.GITHUB_TOKEN` is used for container registry authentication. The workflow has `packages: write` permissions at workflow level, which is appropriate for the GoReleaser push. However, the `docker/login-action` is using the generic GITHUB_TOKEN rather than a dedicated `CR_PAT` (classic Personal Access Token) with only `packages: write` scope. GITHUB_TOKEN is subject to GitHub's automatic token rotation and cannot be used for cross-regional registry pushes or long-lived CI setups.

**CWE:** CWE-284 (but low severity given GitHub's token scoping)

---

## D9 — CI Workflow Has Insecure `continue-on-error` on Load Gate

**File:** `.github/workflows/ci.yml:52-57`

```yaml
- name: Writers-under-load gate
  continue-on-error: true
  env:
    DM_DB_GATE: "1"
    DM_DB_GATE_VERBOSE: "1"
  run: go test -run TestStore_ConcurrentWrites_BaselineGate -v ./internal/db/
```

**Why it's a vulnerability:**

The `continue-on-error: true` flag allows this performance regression gate to fail silently. A developer could introduce a regression in the deployment store's write throughput and still get a green CI. The comment explains it's a temporary measure for baseline capture, but this is a known security-relevant drift risk in database write paths.

**CWE:** CWE-754 (Improper Check for Unusual or Exceptional Conditions)

---

## D10 — go.mod Uses `+incompatible` for Docker SDK

**File:** `go.mod:9`

```go
github.com/docker/docker v28.5.2+incompatible
```

**Why it's a vulnerability:**

The `+incompatible` suffix indicates that `github.com/docker/docker` v28.x does NOT have a valid go.mod entry for the major version being imported (it's a v1/v2 module that hasn't adopted proper go.mod versioning yet). Using `+incompatible` bypasses the Go module compatibility check and can lead to subtle dependency resolution bugs, missing API surface, or unexpected behavior. The package was likely retrieved via the proxy without a proper go.mod declaration.

**CWE:** CWE-1104 (Use of Unmaintained Third-Party Components)

---

## D11 — Missing Trivy Scan in CI Workflow (Before Release)

**File:** `.github/workflows/ci.yml:406-415` (Docker build job)

```yaml
docker:
  name: Docker
  needs: [test, test-react, lint]
  runs-on: ubuntu-latest
  if: github.event_name == 'push'
  steps:
    - uses: actions/checkout@v6

    - name: Build Docker image
      run: docker build -t deploymonster:latest .
```

**Why it's a vulnerability:**

The `docker` job builds a container image but does NOT scan it for vulnerabilities. The Trivy scan in `release.yml` only runs on the final GHCR image after GoReleaser pushes it. This means the local `deploymonster:latest` image built in CI is never scanned. An attacker who compromises the build pipeline could introduce a malicious image without detection.

The release workflow correctly scans the GHCR image, but the CI build scan is missing as a gate before artifacts are published.

**CWE:** CWE-1104 (Use of Unmaintained Third-Party Components)

---

## Summary Table

| ID | Category | File | Line(s) | Severity |
|----|----------|------|---------|----------|
| D1 | Docker | docker-compose.yml | 16 | HIGH |
| D2 | Docker | deployments/docker-compose.dev.yaml | 12 | HIGH |
| D3 | Docker | Dockerfile (root) | 40 | HIGH |
| D4 | Docker | deployments/docker-compose.dev.yaml | 1-20 | MEDIUM |
| D5 | Docker | docker-compose.yml | 1-31 | MEDIUM |
| D6 | IaC | docker-compose.postgres.yml | 11-14 | MEDIUM |
| D7 | IaC | monster.example.yaml | 53 | MEDIUM |
| D8 | CI/CD | .github/workflows/release.yml | 46-51 | LOW |
| D9 | CI/CD | .github/workflows/ci.yml | 52-57 | LOW |
| D10 | Dependency | go.mod | 9 | MEDIUM |
| D11 | CI/CD | .github/workflows/ci.yml | 406-415 | MEDIUM |

---

## Positive Findings (Security Strengths)

- `docker-compose.prod.yml` uses `read_only: true`, `no-new-privileges:true`, tmpfs mounts, and resource limits — excellent production hardening.
- The release workflow uses Trivy with SHA-pinned action commit (post-March-2026 supply-chain compromise mitigation), a strong practice.
- GoReleaser SBOM generation via Syft is properly configured.
- gitleaks is deployed with SHA256 verification of the binary, not just a commit SHA — stronger supply chain.
- Docker socket path validation exists in `internal/core/interfaces.go` for marketplace app isolation.
- `deployments/Dockerfile` correctly switches to non-root user (`USER monster`).