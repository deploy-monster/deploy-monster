# Cryptography Security Scan Results

**Scan Date:** 2026-04-14  
**Scanner:** DeployMonster Crypto Security Audit  
**Scope:** JWT, Password Hashing, Secret Encryption, API Keys, Random Generation, TLS Configuration  

---

## Executive Summary

| Category | Findings | Severity |
|----------|----------|----------|
| JWT Security | 1 LOW | 1 Low |
| Password Hashing | 1 INFO | 1 Info |
| Secret Encryption | 0 | None |
| API Key Hashing | 1 MEDIUM | 1 Medium |
| Random Generation | 1 LOW | 1 Low |
| TLS Configuration | 2 MEDIUM | 2 Medium |
| **TOTAL** | **6** | 0 High, 3 Medium, 2 Low, 1 Info |

**Overall Assessment:** The DeployMonster codebase demonstrates strong cryptographic practices with proper use of AES-256-GCM, bcrypt with cost 13, Argon2id, and Ed25519. Most findings are configuration-related or minor improvements rather than critical vulnerabilities.

---

## 1. JWT Signing Security

### JWT-001: Algorithm Security - SECURE
**File:** `internal/auth/jwt.go` (lines 116, 128, 157)  
**Status:** SECURE  
**Details:**
- Uses `jwt.SigningMethodHS256` (HMAC-SHA256) consistently
- Explicitly validates signing method during token verification (line 157)
- Implements proper key rotation with 20-minute grace period
- Access tokens expire after 15 minutes, refresh tokens after 7 days

**Strengths:**
```go
// Explicit algorithm verification prevents algorithm confusion attacks
if token.Method != jwt.SigningMethodHS256 {
    return nil, jwt.ErrTokenSignatureInvalid
}
```

**Recommendation:** No action required. Current implementation properly defends against algorithm confusion attacks.

---

### JWT-002: JWT Secret Key Strength - LOW
**File:** `internal/auth/jwt.go` (lines 48-65)  
**Severity:** LOW  
**Details:**
- Secret key is passed as a string parameter to `NewJWTService`
- No validation on minimum key length (HS256 requires 256+ bits)
- Key strength depends on deployment configuration

**Evidence:**
```go
func NewJWTService(secret string, previousSecrets ...string) *JWTService {
    // No validation that secret meets minimum entropy requirements
    return &JWTService{
        secretKey: []byte(secret),  // Could be weak if config is poor
        // ...
    }
}
```

**Recommendation:**
1. Add minimum key length validation (32+ bytes for HS256)
2. Document key generation requirements in deployment guide
3. Consider rejecting keys below 256 bits of entropy

**Remediation:**
```go
const MinJWTSecretLength = 32

func NewJWTService(secret string, previousSecrets ...string) (*JWTService, error) {
    if len(secret) < MinJWTSecretLength {
        return nil, fmt.Errorf("JWT secret must be at least %d bytes", MinJWTSecretLength)
    }
    // ...
}
```

---

### JWT-003: Token Revocation Implementation - SECURE
**File:** `internal/auth/jwt.go` (lines 166-194)  
**Status:** SECURE  
**Details:**
- Properly implements access token revocation via JTI (JWT ID) tracking
- Revocation entries have TTL matching token expiration
- Cleanup of expired revocations is automatic via BBolt TTL

---

## 2. Password Hashing Security

### PWD-001: bcrypt Configuration - INFO
**File:** `internal/auth/password.go` (lines 1-50)  
**Severity:** INFORMATIONAL  
**Status:** SECURE with consideration  
**Details:**
- Uses `golang.org/x/crypto/bcrypt` with cost 13
- Cost 13 provides good security/performance balance (~250ms hash time)
- Implements proper password strength validation

**Evidence:**
```go
const bcryptCost = 13

func HashPassword(password string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
    // ...
}
```

**OWASP Recommendation:**
- Current cost 13 is appropriate for most deployments
- OWASP recommends minimum cost 10 for 2024
- Consider cost 14 for high-security environments

**Note:** bcrypt truncates passwords at 72 bytes. Passwords longer than this are silently truncated, which may cause confusion for users with very long passwords.

---

### PWD-002: Password Strength Validation - SECURE
**File:** `internal/auth/password.go` (lines 27-49)  
**Status:** SECURE  
**Details:**
- Minimum 8 characters (configurable)
- Requires uppercase, lowercase, and digit
- Missing special character requirement

**Recommendation:** Consider adding special character requirement for enhanced security:
```go
var hasSpecial bool
for _, r := range password {
    if unicode.IsPunct(r) || unicode.IsSymbol(r) {
        hasSpecial = true
        break
    }
}
```

---

## 3. Secret Encryption (AES-256-GCM)

### ENC-001: Vault Implementation - SECURE
**File:** `internal/secrets/vault.go` (lines 1-123)  
**Status:** SECURE  
**Details:**
- Uses AES-256-GCM with 256-bit keys (32 bytes)
- Properly uses Argon2id for key derivation
- Per-deployment salt (32 bytes) prevents cross-deployment attacks
- 128-bit random nonces via `crypto/rand`

**Evidence:**
```go
func NewVaultWithSalt(masterSecret string, salt []byte) *Vault {
    key := argon2.IDKey([]byte(masterSecret), salt, 1, 64*1024, 4, 32)
    return &Vault{key: key}
}

func (v *Vault) Encrypt(plaintext string) (string, error) {
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", fmt.Errorf("generate nonce: %w", err)
    }
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    // ...
}
```

**Strengths:**
- Argon2id parameters: time=1, memory=64MB, threads=4 (OWASP recommended)
- Nonce prepended to ciphertext for safe decryption
- Proper error handling without information leakage
- Legacy migration path for existing encrypted data

---

### ENC-002: Legacy Salt Handling - SECURE
**File:** `internal/secrets/vault.go` (lines 20-33)  
**Status:** SECURE (Documented)  
**Details:**
- Hardcoded legacy salt seed exists for upgrade compatibility
- Documented as "deploymonster-vault-salt-v1"
- Only used for migration; new deployments get random 32-byte salts

**Evidence:**
```go
const legacyVaultSaltSeed = "deploymonster-vault-salt-v1"

// LegacyVaultSalt returns the salt used by pre-Phase-2 installs.
// Exported so the secrets module's migration path can construct a
// legacy vault without duplicating the derivation logic.
```

**Assessment:** Properly handled. Legacy salt is necessary for upgrade path and is explicitly documented.

---

## 4. API Key Hashing

### APIKEY-001: SHA-256 for API Key Hashing - MEDIUM
**File:** `internal/auth/apikey.go` (lines 37-41)  
**Severity:** MEDIUM  
**CWE:** CWE-916 (Use of Password Hash With Insufficient Computational Effort)  
**Details:**
- API keys are hashed using SHA-256 (fast, non-password hashing)
- API keys are 66 characters: `dm_` + 64 hex chars (32 bytes entropy)
- Stored hash could be susceptible to rainbow table attacks if database is compromised

**Evidence:**
```go
// HashAPIKey creates a SHA-256 hash of an API key.
func HashAPIKey(key string) string {
    h := sha256.Sum256([]byte(key))
    return hex.EncodeToString(h[:])
}
```

**Risk Assessment:**
- **Likelihood:** Low (requires database compromise)
- **Impact:** High (all API keys exposed)
- **Attack Vector:** Rainbow table or brute force against SHA-256

**Recommendation:** Use bcrypt or Argon2id for API key hashing:
```go
func HashAPIKey(key string) string {
    // Use bcrypt with cost 12 for API keys
    hash, err := bcrypt.GenerateFromPassword([]byte(key), 12)
    if err != nil {
        panic(err) // Should never happen
    }
    return string(hash)
}

func VerifyAPIKey(hash, key string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(key)) == nil
}
```

**Workaround:** High-entropy API keys (32 bytes random) partially mitigate this risk.

---

## 5. Random Number Generation

### RAND-001: crypto/rand Fallback to math/big - LOW
**File:** `internal/core/id.go` (lines 1-55)  
**Severity:** LOW  
**Details:**
- Primary: `crypto/rand` (cryptographically secure)
- Fallback: `math/big` when `crypto/rand` fails (extremely rare)
- Used for GenerateID, GenerateSecret, GeneratePassword

**Evidence:**
```go
func GenerateID() string {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err != nil {
        // Fallback to math/big if crypto/rand fails
        slog.Error("crypto/rand unavailable, using math/big fallback", "error", err)
        for i := range b {
            n, _ := rand.Int(rand.Reader, big.NewInt(256))
            b[i] = byte(n.Int64())
        }
    }
    return hex.EncodeToString(b)
}
```

**Assessment:** 
- The fallback still uses `crypto/rand` via `rand.Int(rand.Reader, ...)` - actually secure
- Logging at ERROR level enables alerting on crypto/rand failures
- No actual security issue, but fallback logic is confusing

**Recommendation:** Clarify the fallback actually still uses crypto/rand, or consider failing hard:
```go
func GenerateID() string {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err != nil {
        // crypto/rand is essential for security - fail hard
        panic(fmt.Sprintf("crypto/rand failure: %v", err))
    }
    return hex.EncodeToString(b)
}
```

---

### RAND-002: CSRF Token Generation - SECURE
**File:** `internal/api/middleware/csrf.go` (lines 73-77)  
**Status:** SECURE  
**Details:**
- Uses 16-byte (128-bit) tokens from `crypto/rand`
- Proper `__Host-` prefix for cookie name
- Secure, SameSite=Lax, HttpOnly=false (required for JS access)

**Evidence:**
```go
func generateCSRFToken() string {
    b := make([]byte, 16)
    _, _ = rand.Read(b)
    return hex.EncodeToString(b)
}
```

---

### RAND-003: Request ID Generation - SECURE
**File:** `internal/api/middleware/requestid.go` (lines 79-90)  
**Status:** SECURE  
**Details:**
- Trace IDs: 16 bytes (128-bit) from crypto/rand
- Span IDs: 8 bytes (64-bit) from crypto/rand
- Proper W3C Trace Context support

---

## 6. TLS Configuration

### TLS-001: TLS Version Configuration - SECURE
**File:** `internal/ingress/module.go` (lines 254-260)  
**Status:** SECURE  
**Details:**
- Minimum TLS version: 1.2 (`tls.VersionTLS12`)
- Supports HTTP/2 and HTTP/1.1
- Uses autocert for Let's Encrypt certificates

**Evidence:**
```go
func (m *Module) tlsConfig() *tls.Config {
    return &tls.Config{
        MinVersion:     tls.VersionTLS12,
        GetCertificate: m.acme.GetCertificate,
        NextProtos:     []string{"h2", "http/1.1"},
    }
}
```

---

### TLS-002: Missing Cipher Suite Configuration - MEDIUM
**File:** `internal/ingress/module.go` (lines 254-260)  
**Severity:** MEDIUM  
**Details:**
- No explicit `CipherSuites` configuration
- Go's default includes some CBC-mode ciphers
- TLS 1.3 is not explicitly preferred

**Risk:**
- Go's default cipher suites include TLS 1.2 CBC ciphers which are less secure than GCM
- No downgrade protection beyond MinVersion

**Recommendation:** Explicitly configure secure cipher suites:
```go
func (m *Module) tlsConfig() *tls.Config {
    return &tls.Config{
        MinVersion:     tls.VersionTLS13, // Prefer TLS 1.3
        MaxVersion:     tls.VersionTLS13,
        GetCertificate: m.acme.GetCertificate,
        NextProtos:     []string{"h2", "http/1.1"},
        // For TLS 1.2 compatibility, use only secure ciphers:
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
            tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
            tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
        },
        PreferServerCipherSuites: true,
    }
}
```

---

### TLS-003: Self-Signed Certificate Generation - MEDIUM
**File:** `internal/ingress/tls.go` (lines 134-177)  
**Severity:** MEDIUM  
**Details:**
- `GenerateSelfSigned` creates ECDSA P-256 certificates for development
- Certificate validity: 365 days
- Wildcard SAN included (`*.domain`)
- Used only in development/testing contexts

**Evidence:**
```go
func GenerateSelfSigned(domain string) (*tls.Certificate, error) {
    key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    // ...
    template := x509.Certificate{
        SerialNumber: serialNumber,
        Subject: pkix.Name{
            Organization: []string{"DeployMonster Dev"},
            CommonName:   domain,
        },
        DNSNames:    []string{domain, "*." + domain},  // Wildcard
        NotBefore:   time.Now(),
        NotAfter:    time.Now().Add(365 * 24 * time.Hour),  // 1 year
        KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
    }
    // ...
}
```

**Assessment:** 
- ECDSA P-256 is appropriate
- 365-day validity is reasonable for dev
- Wildcard certificate is convenient but should be clearly documented as DEV ONLY

**Recommendation:** 
1. Add prominent comments indicating DEV USE ONLY
2. Consider shorter validity (90 days) for dev certs
3. Ensure production never uses self-signed certs

---

### TLS-004: SMTP InsecureSkipVerify Configuration - ACCEPTABLE
**File:** `internal/notifications/smtp.go` (lines 81-86, 173-177, 223-228)  
**Severity:** ACCEPTABLE RISK  
**Details:**
- `InsecureSkipVerify` is configurable via YAML
- Security warning printed when enabled
- Defaults to false (secure)
- Required for some self-signed SMTP relays

**Evidence:**
```go
if s.InsecureSkipVerify {
    fmt.Printf("SECURITY WARNING: SMTP InsecureSkipVerify is enabled for host %s. "+
        "TLS certificate verification is DISABLED.\n", s.Host)
}

// Later:
InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // config opt-in for self-signed relays
```

**Assessment:** Properly implemented as opt-in with clear warnings.

---

## 7. Hardcoded Secrets Detection

### SECRET-001: Legacy Vault Salt Seed - ACCEPTABLE
**File:** `internal/secrets/vault.go` (lines 25)  
**Status:** ACCEPTABLE (Documented Legacy)  
**Details:**
- Hardcoded: `deploymonster-vault-salt-v1`
- Purpose: Migration path for pre-Phase-2 deployments
- New deployments use randomly generated 32-byte salts
- Salt is used for key derivation, not stored directly

**Assessment:** Properly documented legacy code for upgrade compatibility. Not a vulnerability.

---

### SECRET-002: No Hardcoded API Keys/Secrets Found - SECURE
**Files Scanned:** All `.go` files  
**Status:** SECURE  
**Details:**
- No hardcoded JWT secrets found
- No hardcoded database passwords
- No hardcoded API keys
- No hardcoded encryption keys
- Configuration properly externalized

---

## 8. SSH Key Generation

### SSH-001: Ed25519 Key Generation - SECURE
**File:** `internal/api/handlers/sshkeys.go` (lines 40-109)  
**Status:** SECURE  
**Details:**
- Uses Ed25519 (modern, secure elliptic curve)
- Generated with `crypto/rand`
- Proper OpenSSH private key format
- Limit of 50 keys per user enforced

**Evidence:**
```go
pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
// ...
privPEM := pem.EncodeToMemory(&pem.Block{
    Type:  "OPENSSH PRIVATE KEY",
    Bytes: privKey,
})
```

**Strengths:**
- Ed25519 is faster and more secure than RSA
- 128-bit security level
- No known vulnerabilities

---

## Summary of Findings

### High Severity (0)
No high-severity cryptographic vulnerabilities found.

### Medium Severity (3)

| ID | Finding | File | Status |
|----|---------|------|--------|
| APIKEY-001 | API keys use SHA-256 instead of bcrypt | `internal/auth/apikey.go:38` | Open |
| TLS-002 | No explicit cipher suite configuration | `internal/ingress/module.go:254` | Open |
| TLS-003 | Self-signed cert validity could be shorter | `internal/ingress/tls.go:154` | Open |

### Low Severity (2)

| ID | Finding | File | Status |
|----|---------|------|--------|
| JWT-002 | No minimum JWT secret length validation | `internal/auth/jwt.go:48` | Open |
| RAND-001 | crypto/rand fallback logic is confusing | `internal/core/id.go:14` | Open |

### Informational (1)

| ID | Finding | File | Status |
|----|---------|------|--------|
| PWD-001 | bcrypt cost 13 is good, cost 14 would be stronger | `internal/auth/password.go:10` | Info |

---

## Remediation Priorities

### Priority 1 (Immediate)
1. **APIKEY-001**: Replace SHA-256 with bcrypt for API key hashing
   - Effort: Low
   - Impact: High
   - Risk if not fixed: Rainbow table attacks on compromised database

### Priority 2 (This Sprint)
2. **TLS-002**: Configure explicit secure cipher suites
   - Effort: Low
   - Impact: Medium
   - Risk if not fixed: Potential use of weak CBC ciphers

### Priority 3 (Next Sprint)
3. **JWT-002**: Add minimum JWT secret length validation
4. **RAND-001**: Clarify or remove confusing fallback logic

---

## Cryptographic Strength Summary

| Component | Algorithm | Rating | Notes |
|-----------|-----------|--------|-------|
| JWT Signing | HS256 | Strong | Proper validation, key rotation |
| Password Hashing | bcrypt cost 13 | Strong | OWASP compliant |
| Secret Encryption | AES-256-GCM + Argon2id | Excellent | Per-deployment salt, proper nonces |
| API Key Hashing | SHA-256 | Weak | Should use bcrypt |
| Random Generation | crypto/rand | Strong | CSPRNG, proper fallbacks |
| TLS | Min 1.2 | Good | Should add explicit ciphers |
| SSH Keys | Ed25519 | Excellent | Modern, secure curve |

---

## Compliance Notes

- **OWASP ASVS 4.0**: V6.2 (cryptography) - Mostly Compliant
- **NIST 800-63B**: Password requirements - Compliant
- **PCI DSS 4.0**: Strong cryptography - Compliant (except API key hashing)
- **GDPR Article 32**: Encryption requirements - Compliant

---

## Appendix: Recommended Configurations

### Secure JWT Secret Generation
```bash
# Generate a 256-bit JWT secret
openssl rand -base64 32
```

### Secure Cipher Suite Configuration
See TLS-002 remediation for Go cipher suite configuration.

### API Key Hashing Migration
When migrating from SHA-256 to bcrypt:
1. Maintain both hash types during transition
2. Re-hash on API key use
3. Remove SHA-256 support after transition period

---

*Report generated by DeployMonster Security Scanner v1.0*
