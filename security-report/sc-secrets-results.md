# sc-secrets Security Scan Report

## Executive Summary

The DeployMonster codebase has been scanned for hardcoded secrets. **No critical production secrets were found.** All detected patterns are either:
1. Test fixtures with obvious fake/test values
2. Legacy hardcoded values intentionally documented for migration purposes
3. Example configuration files with placeholder values

---

## Detailed Findings

### SEC-001: Hardcoded Legacy Vault Salt Seed
**Severity:** Medium  
**Confidence:** 95%  
**File:** `internal/secrets/vault.go`  
**Line:** 25  
**Secret Type:** Cryptographic Salt Seed  

**Masked Secret Value:** `deploymonster-vault-salt-v1`

**Details:**
This is a hardcoded salt seed used for backward compatibility with pre-Phase-2 installations. The salt is SHA256-hashed and truncated to 16 bytes before use. The code includes a migration path that re-encrypts secrets with a newly generated per-deployment salt on first boot.

**Code:**
```go
const legacyVaultSaltSeed = "deploymonster-vault-salt-v1"
```

**Remediation:**
- This is intentionally documented and has a migration path
- Consider deprecating and removing legacy support in a future major version
- Ensure migration runs automatically and removes legacy-encrypted data

---

### SEC-002: Test Credentials in E2E Tests
**Severity:** Low  
**Confidence:** 100%  
**File:** `web/e2e/helpers.ts`  
**Line:** 10  
**Secret Type:** Test Password  

**Masked Secret Value:** `TestPass123!`

**Details:**
Test credentials used for end-to-end testing. These are legitimate test fixtures, not production secrets.

**Code:**
```typescript
export const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};
```

**Remediation:**
- No action required - legitimate test fixture

---

### SEC-003: Test JWT Secret in Auth Tests
**Severity:** Low  
**Confidence:** 100%  
**File:** `internal/auth/jwt_test.go`  
**Line:** 7  
**Secret Type:** JWT Signing Key  

**Masked Secret Value:** `test-secret-key-at-least-32-bytes-long!`

**Details:**
Hardcoded test JWT secret used for unit tests. This is a legitimate test fixture.

**Code:**
```go
const testSecret = "test-secret-key-at-least-32-bytes-long!"
```

**Remediation:**
- No action required - legitimate test fixture

---

### SEC-004: Database Connection Strings in Test Files
**Severity:** Low  
**Confidence:** 100%  
**Files:** Multiple test files  
**Secret Type:** Database Credentials  

**Examples Found:**
- `.github/workflows/ci.yml:347`: `postgres://deploymonster:deploymonster@localhost:5432/deploymonster_test?sslmode=disable`
- `internal/db/postgres_test.go:2100`: `postgres://nobody:nobody@127.0.0.1:1/nonexistent?sslmode=disable`
- `internal/topology/topology_test.go:723`: `mongodb://u:p@mydb:27017/d`

**Details:**
Test database connection strings using localhost/test credentials. These are legitimate test fixtures.

**Remediation:**
- No action required - legitimate test fixtures using local/test databases

---

### SEC-005: Mock API Keys and Tokens in Tests
**Severity:** Low  
**Confidence:** 100%  
**Files:** Various `*_test.go` files  
**Secret Type:** API Keys, Tokens  

**Examples Found:**
- `internal/build/build_final_test.go:19`: `token: "ghp_abc123"` (GitHub token pattern)
- `internal/core/config_compat_test.go:162`: `JoinToken: "SWMTKN-1-xxx"`
- `internal/core/config_compat_test.go:167`: `GitHubClientSecret: "gh-secret"`

**Details:**
Mock API keys and tokens used in unit tests. These follow test naming patterns.

**Remediation:**
- No action required - legitimate test fixtures

---

### SEC-006: Certificate Placeholder in Test
**Severity:** Low  
**Confidence:** 100%  
**File:** `internal/api/handlers/certificates_handler_test.go`  
**Line:** 58  
**Secret Type:** PEM Private Key (Placeholder)  

**Masked Secret Value:** `-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----`

**Details:**
Test certificate uses truncated PEM placeholder (`MIIE...`) which is explicitly allowlisted in `.gitleaks.toml`.

**Remediation:**
- No action required - legitimate placeholder

---

### SEC-007: Example AWS Key in Documentation
**Severity:** Info  
**Confidence:** 100%  
**File:** `CHANGELOG.md`  
**Line:** 219  
**Secret Type:** AWS Access Key (Documented Example)  

**Masked Secret Value:** `AKIAIOSFODNN7EXAMPLE`

**Details:**
This is AWS's official documented example key, used in the changelog to describe the gitleaks allowlist configuration.

**Remediation:**
- No action required - documented example key from AWS documentation

---

### SEC-008: .env.example Placeholder Values
**Severity:** Info  
**Confidence:** 100%  
**File:** `.env.example`  
**Secret Type:** Configuration Placeholders  

**Details:**
The `.env.example` file contains placeholder values:
- `MONSTER_SECRET=change-me-to-a-random-32-char-string`
- `# STRIPE_SECRET_KEY=sk_live_...` (commented)

**Remediation:**
- No action required - proper use of example file with clear placeholders

---

## Summary Statistics

| Category | Count |
|----------|-------|
| Critical Secrets Found | 0 |
| High Severity | 0 |
| Medium Severity | 1 (intentional legacy salt with migration) |
| Low Severity | 5 (all legitimate test fixtures) |
| Info | 2 (documentation/examples) |
| **Total Issues** | **0 production secrets** |

---

## Security Assessment

### Good Practices Observed:
1. **Gitleaks configuration** (`.gitleaks.toml`) properly allowlists test files and documented examples
2. **No real .env files** committed to repository
3. **No private keys** (.pem, .key files) in repository
4. **Test fixtures** follow clear naming conventions (test-, fake-, dummy-, example-)
5. **Legacy salt** is intentionally documented with migration path
6. **Database credentials** only exist in test files pointing to localhost/test instances

### Recommendations:
1. **Verify gitleaks CI integration** is active to prevent accidental commits of real secrets
2. **Periodic review** of the gitleaks allowlist to ensure no abuse
3. **Consider adding pre-commit hooks** for secret scanning
4. **Document** the legacy salt deprecation timeline

---

## Conclusion

**No issues found by sc-secrets in production code.**

All detected patterns are either:
- Legitimate test fixtures with obvious fake values
- Intentionally documented legacy values with migration paths
- Example configuration files with placeholder values

The codebase follows good security practices for secret management.
