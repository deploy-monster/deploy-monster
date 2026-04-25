# sc-business-logic Results

## Summary
Business logic flaw security scan.

## Findings

### Finding: BL-001
- **Title:** Deploy Approval Workflow May Be Bypassed by Direct API Calls
- **Severity:** Low
- **Confidence:** 60
- **File:** internal/api/router.go:628-630
- **Description:** If the deploy approval workflow is not enforced in the deploy trigger handler itself, a direct API call to `/api/v1/apps/{id}/deploy` could bypass pending approvals.
- **Remediation:** Ensure the deploy trigger handler checks for active approval requirements before executing.

### Finding: BL-002
- **Title:** Marketplace Templates May Contain Unsafe Defaults
- **Severity:** Low
- **Confidence:** 55
- **File:** internal/markplace/
- **Description:** Marketplace templates are community or built-in Docker Compose definitions. If templates contain privileged containers or dangerous volume mounts, users may deploy insecure configurations.
- **Remediation:** Validate marketplace templates against a security policy before deployment.

## Positive Security Patterns Observed
- Deploy freeze mechanism exists
- Approval workflow endpoints present
- Bulk operations require authentication
- App transfer requires SuperAdmin
