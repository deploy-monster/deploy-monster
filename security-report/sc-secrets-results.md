# SC-Secrets Results — DeployMonster

## Summary
No production secrets, API keys, tokens, or private keys were found hardcoded in source code.
All sensitive configuration is loaded via environment variables or auto-generated at runtime.

## Scan Scope
- Searched: `**/*.go`, `**/*.yaml`, `**/*.yml`, `**/*.json`, `**/*.md`, `**/*.sh`, `**/*.env*`
- Patterns: AWS keys, GitHub tokens, Stripe keys, Slack tokens, generic secret assignments, private key blocks, connection strings

## Findings

### Finding: SECRET-001 — Test-only credentials in test files (Low)
- **File:** Multiple `*_test.go` files
- **Description:** Test fixtures contain placeholder tokens such as `"test-token"`, `"ghp_abc123"`, `"glpat-xyz"`, `"wrong-secret"`, `"AKIAIOSFODNN7EXAMPLE"`.
- **Impact:** None — these are clearly test/sandbox values and do not grant access to production services.
- **Remediation:** N/A (acceptable test data)

### Finding: SECRET-002 — Auto-generated admin credentials file (Low)
- **File:** `.credentials` (runtime-generated, gitignored)
- **Description:** `internal/auth/module.go:128` writes auto-generated admin credentials to a `.credentials` file with `0600` permissions. The file is listed in `.gitignore` and `.gitleaks.toml` allowlist.
- **Impact:** Low — file is protected by restrictive permissions and excluded from version control.
- **Remediation:** Ensure `.credentials` is never copied into Docker images or backups without encryption.

### Finding: SECRET-003 — Hardcoded private key block in test (Low)
- **File:** `internal/api/handlers/certificates_handler_test.go:58`
- **Description:** A PEM private key block is embedded in a test fixture (`"-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----")`).
- **Impact:** None — test-only dummy key.
- **Remediation:** N/A

### Finding: SECRET-004 — Legacy vault salt seed is hardcoded (Low)
- **File:** `internal/secrets/vault.go:25`
- **Description:** `legacyVaultSaltSeed = "deploymonster-vault-salt-v1"` is a hardcoded string used to derive the legacy vault salt. It is retained only for upgrade paths (pre-Phase-2 installs) and is not used for new deployments.
- **Impact:** Low — new installs generate a random per-deployment salt via `GenerateVaultSalt()`.
- **Remediation:** Document migration timeline to remove legacy support.

## Verdict
No issues found by sc-secrets in production code.
