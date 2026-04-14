# RCE Security Scan Report

**Scan Date:** 2026-04-14  
**Scanner:** sc-rce  
**Target:** DeployMonster Go Codebase

---

## Executive Summary

Scan analyzed the DeployMonster codebase for Remote Code Execution vulnerabilities.

**Overall Findings:**
- **Critical Severity:** 0 issues
- **High Severity:** 1 issue
- **Medium/Low Severity:** 1 informational finding

---

## RCE-001: Command Injection via Unsanitized Build Arguments

**Severity:** High  
**Confidence:** 75%  
**CWE:** CWE-78 (OS Command Injection)  
**File:** `internal/build/builder.go`  
**Line:** 377-378

### Description

The `dockerBuild` function accepts build arguments via the `buildArgs map[string]string` parameter without proper sanitization:

```go
for k, v := range buildArgs {
    args = append(args, "--build-arg", k+"="+v)
}
```

### Attack Scenario

If an attacker can control the `buildArgs` values, they could inject malicious content.

### Mitigating Factors

1. Uses `exec.CommandContext` with argument lists rather than shell string concatenation
2. Build args are sourced from `opts.EnvVars` which may have validation upstream

### Remediation

1. **Validate build argument keys and values:**
```go
var safeArgKey = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

for k, v := range buildArgs {
    if !safeArgKey.MatchString(k) {
        return fmt.Errorf("invalid build arg key: %s", k)
    }
    if strings.ContainsAny(v, "\x00\n\r") {
        return fmt.Errorf("invalid build arg value for %s", k)
    }
    args = append(args, "--build-arg", k+"="+v)
}
```

---

## RCE-002: Command Execution in Container Exec Handler (Informational)

**Severity:** Medium  
**Confidence:** 60%  
**CWE:** CWE-78  
**File:** `internal/api/handlers/exec.go`  
**Line:** 186-195, 224-232

**Description:**
The `Exec` handler allows users to execute commands inside containers with a blocklist-based safety check.

**Observation:**
Blocklists are inherently incomplete and can be bypassed with variations.

**Recommendation:**
1. Implement comprehensive command parsing
2. Use an allowlist approach instead of blocklist
3. Consider using seccomp profiles

---

## Conclusion

The DeployMonster codebase demonstrates generally secure coding practices:
1. **Proper use of exec.Command:** Arguments are passed as arrays
2. **Input validation:** Multiple layers of validation exist
3. **Path traversal protection:** Sanitization functions are in place

The primary concern is **RCE-001** which should be addressed.
