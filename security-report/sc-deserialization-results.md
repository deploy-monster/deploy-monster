# SC-Deserialization Results — DeployMonster

## Summary
No insecure deserialization vulnerabilities were found. The application uses safe deserialization patterns: JSON into explicit structs, YAML into Go structs with no arbitrary object construction, and no `gob`, pickle, ObjectInputStream, BinaryFormatter, or PHP `unserialize` equivalents for untrusted data.

## Findings

No active insecure deserialization findings remain in the reviewed paths.

## Resolved / Revalidated Items

### DESER-001: User-Submitted Compose YAML Parser DoS Hardening
- **Previous Severity:** Low
- **Status:** RESOLVED / HARDENED
- **Files:** `internal/compose/parser.go`, `internal/api/handlers/compose.go`, `internal/marketplace/validator.go`
- **Notes:** Compose parsing now rejects YAML documents larger than 1 MiB at parser level. Direct YAML stack deploys detect oversize bodies and return 413 instead of silently truncating. Marketplace template validation rejects oversized compose YAML before unmarshaling.

### DESER-002: JSON Unmarshal Used For API And KV Data
- **Severity:** Info
- **Status:** SAFE
- **Files:** `internal/api/handlers/*.go`, `internal/db/bolt.go`
- **Notes:** Standard Go JSON unmarshaling into known structs is safe. Reviewed update paths use DTO structs and explicit field copies.

### DESER-003: No Dangerous Native Object Deserialization
- **Severity:** Info
- **Status:** SAFE
- **File:** N/A
- **Notes:** The codebase does not use Go `encoding/gob` for untrusted data and has no language equivalents of pickle/ObjectInputStream/BinaryFormatter/PHP unserialize.

## Positive Security Patterns Observed
- Global API body limit is 10 MB.
- External webhook body limit is 1 MB.
- Compose parser has a 1 MiB document limit.
- YAML is unmarshaled into concrete structs rather than arbitrary object graphs.
- Fuzz tests exist for config and marketplace validator paths.

## Verdict
No active insecure deserialization findings remain.
