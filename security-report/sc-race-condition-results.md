# sc-race-condition Results

## Summary
Race condition and TOCTOU security scan.

## Findings

### Finding: RC-001
- **Title:** Concurrent App Operations Without Distributed Locking
- **Severity:** Medium
- **Confidence:** 65
- **File:** internal/api/router.go (multiple app mutation endpoints)
- **Description:** Multiple endpoints that mutate the same app (deploy, restart, scale, env update) can be called concurrently. While SQLite has row-level locking, there is no explicit distributed lock or optimistic concurrency control for composite operations (e.g., deploy while scaling).
- **Remediation:** Implement optimistic locking with version numbers on app records, or use a distributed lock for composite operations.

### Finding: RC-002
- **Title:** BBolt Concurrent Write Risk
- **Severity:** Low
- **Confidence:** 70
- **File:** internal/db/bolt.go
- **Description:** BBolt allows only one writer at a time. Concurrent writes from multiple goroutines could block or timeout under high load.
- **Remediation:** Ensure all BBolt writes use a consistent mutex or channel-based serialization.

## Positive Security Patterns Observed
- `concurrent_writes_gate_test.go` exists, indicating awareness of concurrency issues
- SQLite transactions used for atomic operations
- `sync.Mutex` patterns observed in core modules
