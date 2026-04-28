# sc-docker Results

## Summary
Docker security scan.

## Findings

### Finding: DOCKER-001
- **Title:** Docker Client v28.5.2 Known Vulnerabilities
- **Severity:** High
- **Confidence:** 85
- **File:** go.mod:9
- **Description:** Known AuthZ bypass and plugin privilege escalation in Docker v28.x.
- **Remediation:** Upgrade to Docker client v29+ when available.

### Finding: DOCKER-002
- **Title:** Build Pipeline Runs Without Capability Dropping
- **Severity:** Medium
- **Confidence:** 60
- **File:** internal/build/builder.go
- **Description:** Docker builds are executed with default capabilities. No `--cap-drop=ALL` or seccomp profiles are applied.
- **Remediation:** Apply least-privilege Docker build options. Run builds in isolated environments.

### Finding: DOCKER-003
- **Title:** Dockerfile USER Not Set (Assumed)
- **Severity:** Medium
- **Confidence:** 70
- **File:** Dockerfile (root user likely)
- **Description:** The project generates Dockerfiles for user apps. If these generated Dockerfiles do not include a `USER` directive, containers run as root.
- **Remediation:** Ensure all generated Dockerfiles include a non-root `USER` directive.

## Positive Security Patterns Observed
- Docker build args validated against injection
- Image tags validated against format patterns
- Build timeout enforced
- `--force-rm` used to clean up intermediate containers
