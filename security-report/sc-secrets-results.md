# SC-Secrets Results — DeployMonster

## Summary
No production secrets, API keys, tokens, or private keys were found hardcoded in source code.
All sensitive configuration is loaded via environment variables or auto-generated at runtime.

## Scan Scope
- Searched: `**/*.go`, `**/*.yaml`, `**/*.yml`, `**/*.json`, `**/*.md`, `**/*.sh`, `**/*.env*`
- Patterns: AWS keys, GitHub tokens, Stripe keys, Slack tokens, generic secret assignments, private key blocks, connection strings

## Findings

No active production secret findings remain in the scanned source paths.

### Finding: SECRET-001 — Test-only credentials in test files (Low)
- **File:** Multiple `*_test.go` files
- **Description:** Test fixtures contain placeholder tokens such as `"test-token"`, `"ghp_abc123"`, `"glpat-xyz"`, `"wrong-secret"`, `"AKIAIOSFODNN7EXAMPLE"`.
- **Impact:** None — these are clearly test/sandbox values and do not grant access to production services.
- **Remediation:** N/A (acceptable test data)

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

## Resolved Findings

### SECRET-002 — Legacy Auto-Generated Admin Credentials File Removed
- **Status:** Resolved
- **Files:** `internal/auth/module.go`, `scripts/install.sh`
- **Description:** Current first-run setup no longer writes a runtime `.credentials` file. The app logs an auto-generated bootstrap password once, unsets `MONSTER_ADMIN_EMAIL` and `MONSTER_ADMIN_PASSWORD` after use, and removes `/etc/deploymonster/deploymonster.env` if it contains bootstrap admin credentials. The installer may write bootstrap credentials into that env file transiently for first start, but the application cleanup removes it after successful setup.
- **Remediation:** N/A

### SECRET-005 — Marketplace Templates Used Weak Secret Defaults
- **Status:** Resolved
- **Files:** `internal/marketplace/registry.go`, `internal/marketplace/registry_test.go`, `internal/marketplace/templates_extra_test.go`
- **Description:** Built-in Docker Compose templates included weak fallback values such as `${SECRET:-changeme}` and hardcoded database passwords in scalar YAML values or connection strings.
- **Remediation:** Template registration now sanitizes weak sensitive defaults across `${VAR:-weak}`, `PASSWORD: weak`, URL userinfo passwords, and query-string password parameters. Regression tests assert no weak defaults remain after built-ins are loaded.

## Verdict
No issues found by sc-secrets in production code.
