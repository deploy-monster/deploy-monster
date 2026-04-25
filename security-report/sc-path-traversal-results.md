# sc-path-traversal Results

## Summary
Path traversal and LFI/RFI security scan.

## Findings

### Finding: PT-001
- **Title:** File Browser Endpoint May Allow Path Traversal
- **Severity:** Medium
- **Confidence:** 65
- **File:** internal/api/router.go:366
- **Description:** `GET /api/v1/apps/{id}/files` provides file browser access to containers. The handler implementation was not fully reviewed, but file listing APIs on container filesystems are a classic path traversal vector if the requested path is not strictly validated.
- **Remediation:** Ensure the file browser handler strictly validates paths against the container's root filesystem and rejects `..`, absolute paths, and symlink escapes.

### Finding: PT-002
- **Title:** Backup Download Uses User-Controlled Key
- **Severity:** Low
- **Confidence:** 60
- **File:** internal/api/router.go:495
- **Description:** `GET /api/v1/backups/{key}/download` uses a user-provided key to locate backup files. If the key is not sanitized, path traversal could expose arbitrary files on the backup storage.
- **Remediation:** Validate backup keys against a strict alphanumeric pattern. Ensure backup storage backend resolves keys safely without filesystem traversal.

## Positive Security Patterns Observed
- `filepath.Join` used consistently instead of string concatenation
- Build directory created with `0750` permissions
- No direct user input used in `os.OpenFile` for filesystem access observed in reviewed code
