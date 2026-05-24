# sc-path-traversal Results

## Summary
Path traversal and LFI/RFI security scan.

## Findings

No active path traversal findings remain in the reviewed working tree.

## Resolved Findings

### Resolved: PT-003
- **Title:** Topology Deployment Path Components Accepted Encoded Traversal
- **Severity:** Medium
- **Confidence:** 80
- **File:** internal/api/handlers/topology.go
- **Status:** RESOLVED
- **Description:** Topology save/load/compile/deploy used project and environment identifiers in storage keys and deployment working directories, but validation was inconsistent and deploy only checked a single decoded traversal pattern.
- **Remediation Applied:** `projectId` and `environment` now share strict path-component validation across topology save, load, compile, and deploy. The validator rejects empty values, whitespace, URL-encoded traversal, double-encoded traversal, separators, dots, backslashes, colons, and other non `[A-Za-z0-9_-]` characters.

### Resolved: PT-001
- **Title:** File Browser Path Validation Hardened
- **Severity:** Medium
- **Confidence:** 70
- **File:** internal/api/handlers/filebrowser.go
- **Status:** RESOLVED / HARDENED
- **Description:** `GET /api/v1/apps/{id}/files` currently returns a structural response and does not read container files, but it is a high-risk future extension point because the request path is user-controlled.
- **Remediation Applied:** File-browser path validation now URL-decodes twice before checking traversal, rejects null/control bytes and Windows drive prefixes, and keeps `..` traversal blocked after path normalization.

### Resolved: PT-002
- **Title:** Backup Download Key Validation Hardened
- **Severity:** Low
- **Confidence:** 75
- **File:** internal/api/handlers/backups.go
- **Status:** RESOLVED
- **Description:** `GET /api/v1/backups/download/{key...}` accepts a user-controlled key. The local storage implementation already enforces root containment and symlink rejection, but the API boundary previously only checked tenant prefix and simple traversal markers.
- **Remediation Applied:** Backup restore/download now require strict backup keys before storage access: matching tenant prefix, no encoded rewrites, no empty/dot segments, no trailing slash, no repeated slash, and only `[A-Za-z0-9._-/]` characters.

### Resolved: PT-004
- **Title:** Custom Dockerfile Path Could Escape Build Context
- **Severity:** Medium
- **Confidence:** 80
- **File:** internal/build/builder.go
- **Status:** RESOLVED
- **Description:** Build options can select a custom Dockerfile path. The previous path join did not explicitly reject absolute, dot, null-byte, or parent traversal paths before passing `-f` to `docker build`.
- **Remediation Applied:** Custom Dockerfile paths now resolve through `resolveDockerfilePath`, which requires a relative path and verifies with `filepath.Rel` that the final path stays inside the generated build directory.

## Positive Security Patterns Observed
- `filepath.Join` used consistently instead of string concatenation
- Topology deployment path components now use centralized strict validation before persistence, compilation, or deployment
- Backup storage performs root-containment checks with `filepath.Rel` and rejects symlink targets
- File-browser path validation rejects encoded and double-encoded traversal attempts
- Custom Dockerfile paths are constrained to the build context before `docker build`
- Build directory created with `0750` permissions
- No direct user input used in `os.OpenFile` for filesystem access observed in reviewed code
