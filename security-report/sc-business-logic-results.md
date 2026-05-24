# sc-business-logic Results

## Summary
Business logic flaw security scan.

## Findings

### BL-001: Deploy Approval Workflow Not Enforced By Deploy Trigger
- **Severity:** Low-Medium
- **Confidence:** 75
- **Files:** `internal/api/handlers/deploy_trigger.go`, `internal/api/handlers/deploy_approval.go`
- **Description:** Deploy approval endpoints exist and maintain tenant-scoped pending requests, but `POST /api/v1/apps/{id}/deploy` does not consult approval state or an approval-required policy before starting an image deploy or build pipeline. Deploy freeze is enforced in the trigger handler; approval workflow enforcement is not yet wired into that path.
- **Remediation:** Add a persisted tenant/app deploy-approval policy, have deploy triggers create or require approved requests before execution, and make approval consume/launch the exact deployment it reviewed.

### BL-002: Marketplace Templates May Contain Unsafe Non-Secret Defaults
- **Severity:** Low
- **Confidence:** 55
- **File:** `internal/marketplace/`
- **Description:** Marketplace templates are community or built-in Docker Compose definitions. Weak secret defaults are sanitized at registry insertion time, but privileged containers, broad bind mounts, and application-specific unsafe defaults should continue to be reviewed as new templates are added.
- **Remediation:** Keep marketplace template validation in CI and extend the policy for privileged containers, host mounts, and exposed admin services.

## Positive Security Patterns Observed
- Deploy freeze mechanism exists
- Manual app, compose stack, marketplace, and topology deploys now enforce active deploy-freeze windows before changing app status or creating deployment resources
- Approval workflow endpoints present
- Bulk operations require authentication
- App transfer requires SuperAdmin
- Marketplace registry sanitizes weak secret fallbacks and hardcoded weak Compose credentials

## Resolved Findings

### BL-003: Deploy Surfaces Ignored Active Freeze Windows
- **Status:** Resolved
- **Files:** `internal/api/handlers/deploy_trigger.go`, `internal/api/handlers/compose.go`, `internal/api/handlers/marketplace_deploy.go`, `internal/api/handlers/topology.go`, `internal/api/router.go`
- **Evidence:** Manual app deploy, compose stack deploy, marketplace deploy, and topology deploy now read tenant freeze windows and return `423 Locked` while an active window is in effect, before app records, deployment state, or topology resources are created.
