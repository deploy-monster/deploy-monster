# Secrets Exposure and Data Protection Findings

**Phase 2: HUNT** - Security Audit Report
**Date**: 2026-04-13
**Auditor**: Security Audit Phase 2

---

## FINDING 1: SQLite Database Without Encryption

**File**: `internal/db/sqlite.go`
**Line**: 30

**Code Snippet**:
```go
dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL", path)
```

**Why It's a Vulnerability**:
The SQLite database is opened without encryption. The DSN string does not include any encryption parameters. SQLite's CCE (SQLite Encryption Extension) or SQLCipher requires explicit `cipher` pragmas in the DSN. Without encryption at rest, all tenant data, user credentials, secrets, and application data stored in SQLite are readable if the database file is compromised (e.g., disk theft, container escape, unauthorized file access).

**CWE Reference**: CWE-311 - Missing Encryption of Sensitive Data

---

## FINDING 2: BBolt KV Store Without Encryption

**File**: `internal/db/bolt.go`
**Line**: 49

**Code Snippet**:
```go
db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
```

**Why It's a Vulnerability**:
BBolt is an embedded key-value database that does not support encryption at rest. The `bolt.Open` call uses no encryption options. BBolt stores sensitive data including:
- API keys (`bucketAPIKeys`)
- Sessions and refresh tokens (`bucketSessions`, `bucketRevokedTokens`)
- Webhook secrets (`bucketWebhooks`)
- Basic auth credentials (`bucketBasicAuth`)

If an attacker gains read access to the BBolt file, all stored secrets are exposed in plaintext.

**CWE Reference**: CWE-311 - Missing Encryption of Sensitive Data

---

## FINDING 3: Secrets Can Be Stored Without Encryption (Nil Vault Fallback)

**File**: `internal/api/handlers/secrets.go`
**Lines**: 62-68

**Code Snippet**:
```go
// Encrypt the value
encrypted := req.Value
if h.vault != nil {
    enc, err := h.vault.Encrypt(req.Value)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "encryption failed")
        return
    }
    encrypted = enc
}
// stored as plaintext if vault is nil
```

**Why It's a Vulnerability**:
The `SecretHandler.Create` method accepts a nil vault and falls back to storing secrets in plaintext. While the vault should be initialized by the secrets module, the nil-check bypass means that if initialization fails or is bypassed, all secret values are stored as plaintext in the SQLite database. The test at `internal/api/handlers/secrets_handler_test.go:113` explicitly documents this behavior: "vault is nil — value should be stored as-is (no encryption)".

**CWE Reference**: CWE-311 - Missing Encryption of Sensitive Data

---

## FINDING 4: AuthRateLimiter Trusts X-Forwarded-For by Default

**File**: `internal/api/middleware/ratelimit.go`
**Lines**: 29-35

**Code Snippet**:
```go
func NewAuthRateLimiter(bolt core.BoltStorer, rate int, window time.Duration, prefix string, opts ...Option) *AuthRateLimiter {
    rl := &AuthRateLimiter{
        // ...
        trustXFF: true, // default: match original behavior (trust XFF)
    }
```

**Why It's a Vulnerability**:
The `NewAuthRateLimiter` defaults to `trustXFF: true`, meaning it trusts the `X-Forwarded-For` header for client IP identification in rate limiting. If DeployMonster is deployed without a trusted reverse proxy (nginx, Traefik, load balancer), an attacker can spoof the `X-Forwarded-For` header to bypass rate limiting on auth endpoints (`/api/v1/auth/login`, `/api/v1/auth/register`). The code has IP validation (`validateIP` function at line 84), but the default trust behavior before validation creates a window of vulnerability when no proxy exists.

**CWE Reference**: CWE-346 - Origin Validation Error

---

## FINDING 5: Local Backup Storage Has No Encryption

**File**: `internal/backup/local.go`
**Lines**: 24-56

**Code Snippet**:
```go
func NewLocalStorage(basePath string) *LocalStorage {
    _ = os.MkdirAll(basePath, 0750)
    return &LocalStorage{basePath: basePath}
}

func (l *LocalStorage) Upload(_ context.Context, key string, reader io.Reader, _ int64) error {
    // ... path traversal checks ...
    f, err := os.Create(path)
    if err != nil {
        return fmt.Errorf("create backup file: %w", err)
    }
    defer f.Close()
    if _, err := io.Copy(f, reader); err != nil {
        return fmt.Errorf("write backup: %w", err)
    }
    return nil
}
```

**Why It's a Vulnerability**:
`LocalStorage.Upload` writes backup data directly to disk without encryption. Although `BackupConfig.Encryption` exists and defaults to `true` in `internal/core/config.go:424`, the `LocalStorage` implementation ignores this setting entirely. Backups are stored in plaintext and can be read by anyone with filesystem access.

Note: S3 storage uses the AWS SDK which supports server-side encryption, but the upload path does not explicitly enable encryption.

**CWE Reference**: CWE-311 - Missing Encryption of Sensitive Data

---

## FINDING 6: Default Admin Password Is Empty String

**File**: `internal/auth/module.go`
**Lines**: 99-100

**Code Snippet**:
```go
email := getEnvOrDefault("MONSTER_ADMIN_EMAIL", "admin@deploy.monster")
password := getEnvOrDefault("MONSTER_ADMIN_PASSWORD", "")
```

**Why It's a Vulnerability**:
If `MONSTER_ADMIN_PASSWORD` is not set, the default password is an empty string. This means a fresh installation with no environment variable configured will create an admin account with email `admin@deploy.monster` and an empty/blank password. While this is intended for first-run setup, it poses a risk if the registration mode allows open registration or if the admin account is not properly secured during initial setup.

**CWE Reference**: CWE-258 - Empty Password

---

## Summary Table

| Finding | File | Line | CWE | Severity |
|---------|------|------|-----|----------|
| SQLite without encryption | internal/db/sqlite.go | 30 | CWE-311 | High |
| BBolt without encryption | internal/db/bolt.go | 49 | CWE-311 | High |
| Nil vault stores plaintext secrets | internal/api/handlers/secrets.go | 62-68 | CWE-311 | High |
| AuthRateLimiter trusts XFF by default | internal/api/middleware/ratelimit.go | 35 | CWE-346 | Medium |
| LocalStorage backup not encrypted | internal/backup/local.go | 24-56 | CWE-311 | Medium |
| Default admin password is empty string | internal/auth/module.go | 100 | CWE-258 | Low |

---

## Recommendations

1. **SQLite Encryption**: Consider using `github.com/nicbilli/csql` or switching to PostgreSQL for encryption-at-rest support.

2. **BBolt Encryption**: BBolt does not support encryption natively. Consider adding a layer of encryption in the application code before storing sensitive data, or use an encrypted filesystem (LUKS, dm-crypt) for the BBolt data directory.

3. **Secrets Handler**: Remove the nil vault fallback or ensure the vault is always initialized before the secrets handler is used.

4. **Rate Limiter XFF**: Change the default `trustXFF` to `false` unless a trusted proxy is explicitly configured.

5. **Backup Encryption**: Implement encryption in `LocalStorage.Upload` using the `Encryption` config flag, or document that LocalStorage requires filesystem-level encryption.

6. **Admin Password**: Add validation to require `MONSTER_ADMIN_PASSWORD` to be set with minimum complexity if `MONSTER_REGISTRATION_MODE` is not `closed`.
