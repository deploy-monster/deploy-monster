# SC-Deserialization Results — DeployMonster

## Summary
No insecure deserialization vulnerabilities were found. The application uses safe deserialization patterns: `json.Unmarshal`/`json.NewDecoder` for JSON, `yaml.Unmarshal` from `gopkg.in/yaml.v3` for trusted config/compose data, and `gob` is not used. No `pickle`, `ObjectInputStream`, `BinaryFormatter`, or `unserialize` equivalents exist in the Go codebase.

## Findings

### Finding: DESER-001 — `yaml.Unmarshal` used for config and compose files (Low)
- **File:** `internal/core/config.go:212`, `internal/core/config.go:224`, `internal/compose/parser.go:109`, `internal/marketplace/validator.go:128`
- **Severity:** Low
- **Confidence:** 85
- **Vulnerability Type:** CWE-502 (Deserialization of Untrusted Data)
- **Description:** `gopkg.in/yaml.v3` unmarshals YAML into plain structs. In Go, `yaml.v3` does not support arbitrary object instantiation like Python's `yaml.load`; it maps YAML into struct fields and basic types. However, `compose/parser.go` and `marketplace/validator.go` parse user-submitted compose YAML. The parsed data is validated structurally but is still attacker-controlled input.
- **Impact:** Low — Go's `yaml.v3` is not vulnerable to RCE via deserialization. The worst case is a parser panic or excessive memory use.
- **Remediation:** Add YAML size limits and timeout bounds before unmarshaling user-submitted compose files to prevent DoS.
- **References:** https://cwe.mitre.org/data/definitions/502.html

### Finding: DESER-002 — `json.Unmarshal` used extensively for API and BBolt data (Safe)
- **File:** Throughout `internal/api/handlers/*.go`, `internal/db/bolt.go`
- **Severity:** Info
- **Confidence:** 95
- **Description:** Standard Go JSON unmarshaling into known structs is safe. No custom `UnmarshalJSON` methods perform dangerous operations. BBolt entries are unmarshaled into concrete types (`models.APIKey`, `models.Webhook`, etc.).

### Finding: DESER-003 — No pickle, ObjectInputStream, BinaryFormatter, or PHP unserialize (Safe)
- **File:** N/A
- **Severity:** Info
- **Description:** Go does not have `pickle` or `ObjectInputStream`. The codebase does not use `encoding/gob` for untrusted data. All serialization paths use schema-defined formats (JSON, YAML to structs).

## Verdict
No issues found by sc-deserialization in production code.
