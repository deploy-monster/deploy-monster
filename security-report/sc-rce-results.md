# sc-rce Results

## Summary
Remote code execution security scan.

## Findings

### Finding: RCE-001
- **Title:** Container Exec as Intentional RCE Surface
- **Severity:** Info
- **Confidence:** 95
- **File:** internal/api/handlers/exec.go:164, internal/api/ws/terminal.go:163
- **Description:** `POST /api/v1/apps/{id}/exec` and terminal endpoints allow authenticated users to execute arbitrary commands inside their containers. This is an intentional platform feature, not a vulnerability per se, but it represents a significant RCE surface that depends entirely on authentication and authorization controls.
- **Remediation:** Ensure continuous monitoring of exec commands via audit logs. Consider adding a mandatory approval workflow for exec access in high-security environments.

### Finding: RCE-002
- **Title:** Build Pipeline Executes User-Controlled Git Clone and Docker Build
- **Severity:** Medium
- **Confidence:** 80
- **File:** internal/build/builder.go:59-156
- **Description:** The build pipeline clones user-provided Git URLs and builds Docker images. While `ValidateGitURL` and `validateResolvedHost` provide SSRF and DNS rebinding protection, the pipeline still executes `git clone` and `docker build` with user-controlled inputs. A bypass in URL validation could lead to RCE.
- **Remediation:** Run builds in isolated, sandboxed environments (e.g., gVisor, Firecracker, or a separate VM). Drop all unnecessary Docker capabilities during build.

## Positive Security Patterns Observed
- Git URL validation blocks shell metacharacters, private IPs, and DNS rebinding
- Docker build args validated against control characters and flag injection
- Build timeout enforced (30 min default)
- Build directory cleaned up after completion
- Panic recovery in build pipeline prevents crashes
