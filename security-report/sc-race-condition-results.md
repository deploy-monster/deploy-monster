# sc-race-condition Results

## Summary
Race condition and TOCTOU security scan.

## Findings

### RC-001: Concurrent App Operations Without App-Level Coordination
- **Severity:** Medium
- **Confidence:** 65
- **File:** `internal/api/handlers/` (multiple app mutation endpoints)
- **Description:** Multiple endpoints that mutate the same app (deploy, restart, scale, env update) can be called concurrently. While SQLite has row-level locking, there is no explicit distributed lock or optimistic concurrency control for composite operations (e.g., deploy while scaling).
- **Remediation:** Implement optimistic locking with version numbers on app records, or use a distributed lock for composite operations.

### RC-002: KV Read-Modify-Write Paths Can Lose Updates
- **Severity:** Low
- **Confidence:** 70
- **File:** `internal/db/bolt.go`, `internal/api/handlers/`
- **Description:** SQLite-backed KV storage serializes writes safely, so raw concurrent writes are not a corruption risk. A transactional mutation helper now exists, and outbound event webhooks plus redirect rules use it. The remaining risk is other handler-level read-modify-write sequences that still read a list, mutate it in memory, and write it back with separate `Get`/`Set` calls.
- **Remediation:** Migrate remaining list-like buckets, including freeze windows, certificate lists, registry lists, cron jobs, SSH keys, announcements, and similar handlers, to `BoltStore.Mutate` or an equivalent store-level write transaction.

## Resolved Findings

### RC-003: Deploy Approval Double-Processing Race
- **Severity:** Low
- **Confidence:** 85
- **File:** `internal/api/handlers/deploy_approval.go`
- **Status:** Resolved
- **Description:** Approval status checks and mutations now happen under the same mutex lock, so concurrent approve/reject requests cannot both observe `pending` and process the same approval. The handler also tolerates a nil event bus in tests and fallback contexts.
- **Evidence:** `TestDeployApproval_Approve_AlreadyProcessedConflict` and `TestDeployApproval_Reject_AlreadyProcessedConflict` assert processed approvals remain immutable and return `409 Conflict`.

### RC-004: Outbound Event Webhook List Lost Updates
- **Severity:** Low
- **Confidence:** 85
- **Files:** `internal/db/bolt.go`, `internal/api/handlers/event_webhooks.go`
- **Status:** Resolved
- **Description:** Outbound webhook create/delete now use `BoltStore.Mutate`, which loads, modifies, and writes the tenant webhook list inside one SQLite write transaction when the production store is used. This removes the previous `Get`/append/`Set` lost-update window for concurrent webhook changes.
- **Evidence:** `TestBolt_Mutate_UpdatesInsideSingleTransaction` verifies the store primitive and `TestEventWebhookHandler_Create_ConcurrentPreservesAllWebhooks` verifies concurrent webhook creation preserves all entries.

### RC-005: Redirect Rule List Lost Updates
- **Severity:** Low
- **Confidence:** 85
- **Files:** `internal/api/handlers/redirects.go`, `internal/db/bolt.go`
- **Status:** Resolved
- **Description:** Redirect rule create/delete now use the same transaction-scoped mutation helper, closing the previous `Get`/append-or-filter/`Set` window for concurrent app redirect rule changes.
- **Evidence:** `TestRedirectHandler_Create_ConcurrentPreservesAllRules` verifies concurrent rule creation preserves all entries.

## Positive Security Patterns Observed
- `concurrent_writes_gate_test.go` exists, indicating awareness of write concurrency/performance issues
- SQLite transactions used for atomic operations
- `sync.Mutex` patterns observed in core modules
- Deployment versions use `AtomicNextDeployVersion`
- Deploy approval processing now keeps state checks and state transitions under one lock
- `BoltStore.Mutate` is available for transaction-scoped read-modify-write updates
- Outbound event webhooks and redirect rules use transaction-scoped list mutation
