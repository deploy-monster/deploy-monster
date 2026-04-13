# Injection Vulnerability Findings

**Audit Date**: 2026-04-13
**Phase**: 2 - HUNT
**Scope**: SQL Injection, Command Injection, XSS, Header Injection, LDAP Injection, GraphQL Injection, NoSQL Injection

---

## Executive Summary

No critical injection vulnerabilities were found. The codebase demonstrates strong security practices:

- **SQL Injection**: All database queries use parameterized statements (`?` placeholders). No string concatenation in SQL.
- **Command Injection**: Git operations use argument arrays with validation (blacklist patterns + URL validation). Docker commands use controlled argument lists.
- **XSS**: No `innerHTML` or `dangerouslySetInnerHTML` usage found in the React codebase.
- **Header Injection**: All response headers use hardcoded static values; no user-controlled header values.
- **LDAP Injection**: LDAP is not used in this codebase.
- **GraphQL Injection**: GraphQL is not used in this codebase.
- **NoSQL Injection**: MongoDB is defined as a planned future database engine but is not currently implemented.

---

## Detailed Findings

### 1. SQL Injection - NOT VULNERABLE

**Location**: `internal/db/*.go` (all database files)

**Analysis**:
All database operations use the `database/sql` package with parameterized queries:

```go
// internal/db/tenants.go:32
err := s.QueryRowContext(ctx,
    `SELECT id, name, slug, avatar_url, plan_id, COALESCE(owner_id,''), COALESCE(reseller_id,''),
            status, limits_json, metadata_json, created_at, updated_at
     FROM tenants WHERE id = ?`, id,
).Scan(...)
```

No string concatenation with SQL keywords. All user input passed via `?` placeholders.

**CWE**: N/A - Secure

---

### 2. Command Injection - MITIGATED (with observations)

**Location**: `internal/build/builder.go`

**Observation**:
The git clone operation uses `exec.CommandContext` with a URL that can be user-controlled:

```go
// internal/build/builder.go:340
cmd := exec.CommandContext(ctx, "git", args...)
```

**Mitigations in place**:
1. URL validation via `ValidateGitURL()` (line 206-256) blocks shell metacharacters and private IP ranges
2. DNS rebinding protection via `validateResolvedHost()` (line 262-302)
3. Token injection does not use shell - it reconstructs the URL safely (line 363-368)
4. Command arguments are passed as a slice, not via shell

**Residual Risk**: Low - The defense-in-depth URL validation is robust, but command injection cannot be fully ruled out if a valid git URL is used as-is and contains unexpected arguments.

**CWE**: CWE-78 (Command Injection) - Mitigated

---

**Location**: `internal/topology/deployer.go`

**Analysis**:
Docker compose commands use static argument arrays:

```go
// internal/topology/deployer.go:110
cmd := exec.CommandContext(ctx, "docker", "compose", "-f", d.composePath, "config", "--quiet")
```

Arguments are hardcoded; file paths come from internal state, not user input.

**CWE**: N/A - Secure

---

**Location**: `internal/api/handlers/exec.go`

**Analysis**:
Container exec command uses controlled argument passing:

```go
// internal/api/handlers/exec.go:232
output, err := h.runtime.Exec(r.Context(), containerID, cmd)
```

**Security measures**:
1. Blacklist-based pattern detection (blocked patterns like `rm -rf /`, fork bombs, etc.)
2. Command string splitting that treats shell operators as data
3. App ownership verification via `requireTenantApp()`

**Residual Risk**: Low - Blacklist approach may miss novel attack patterns. The defense is strong but not comprehensive.

**CWE**: CWE-78 (Command Injection) - Mitigated

---

### 3. XSS (Cross-Site Scripting) - NOT VULNERABLE

**Location**: `web/src/**/*.{ts,tsx}`

**Analysis**:
Searched for `innerHTML`, `dangerouslySetInnerHTML`, and `document.write` - none found.

React's JSX does not use `innerHTML` directly; the codebase uses standard React patterns.

**CWE**: N/A - Secure

---

### 4. Header Injection - NOT VULNERABLE

**Location**: `internal/api/middleware/middleware.go` (CORS handler)

**Analysis**:
```go
// internal/api/middleware/middleware.go:129
w.Header().Set("Access-Control-Allow-Origin", origin)
```

Origin comes from `r.Header.Get("Origin")` and is validated against an allowlist:

```go
// internal/api/middleware/middleware.go:126-132
for _, allowed := range strings.Split(allowedOrigins, ",") {
    if strings.TrimSpace(allowed) == origin {
        originMatched = true
        w.Header().Set("Access-Control-Allow-Origin", origin)
```

No direct echo of user input without validation.

**CWE**: N/A - Secure

**Location**: `internal/api/handlers/helpers.go`

```go
// internal/api/handlers/helpers.go:224
func safeFilename(name string) string {
    return safeFilenameRe.ReplaceAllString(name, "_")
}
```

`safeFilename()` sanitizes all filename values used in `Content-Disposition` headers, preventing injection.

**CWE**: N/A - Secure

---

### 5. LDAP Injection - NOT PRESENT

**Analysis**: No usage of LDAP libraries found in the codebase.

**CWE**: N/A - Not applicable

---

### 6. GraphQL Injection - NOT PRESENT

**Analysis**: No GraphQL implementation found in the codebase.

**CWE**: N/A - Not applicable

---

### 7. NoSQL Injection - NOT PRESENT

**Analysis**: MongoDB is referenced as a planned database engine type in `internal/topology/types.go:104`:

```go
EngineMongoDB  DatabaseEngine = "mongodb"
```

However, no MongoDB client code exists. The planned PostgreSQL support also uses the same parameterized query patterns.

**CWE**: N/A - Not applicable

---

## Summary Table

| Vulnerability Type | Status | Evidence |
|-------------------|--------|----------|
| SQL Injection | SECURE | All queries use parameterized `?` placeholders |
| Command Injection | MITIGATED | URL validation, argument arrays, blacklist patterns |
| XSS | SECURE | No innerHTML/dangerouslySetInnerHTML usage found |
| Header Injection | SECURE | All headers use hardcoded/static values |
| LDAP Injection | N/A | Not used |
| GraphQL Injection | N/A | Not used |
| NoSQL Injection | N/A | MongoDB not implemented |

---

## Recommendations

1. **Command Injection**: Consider extending the blacklist in `exec.go` to include additional patterns. The URL validation in `builder.go` is already strong.

2. **Defense in Depth**: The git URL validation is comprehensive. Consider if the same level of rigor should apply to other external resource fetching (e.g., fetching remote Dockerfiles or helm charts).

3. **exec Handler**: The blacklist-based `isCommandSafe()` could be supplemented with an allowlist approach for stricter control over which commands are permitted.