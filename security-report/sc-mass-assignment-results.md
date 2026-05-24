# sc-mass-assignment Results

## Summary
Mass assignment security scan.

## Findings

No active mass-assignment finding is verified in the current working tree.

## Revalidated Items

### MA-001: PATCH Handlers May Allow Unintended Field Updates
- **Previous Severity:** Medium
- **Status:** NOT VERIFIED / CURRENTLY CONTROLLED
- **Files:** `internal/api/handlers/sessions.go`, `internal/api/handlers/app_update.go`, `internal/api/handlers/tenant_settings.go`
- **Notes:** Reviewed PATCH paths decode into explicit DTO structs and then copy allowlisted fields onto loaded store models. `PATCH /api/v1/auth/me` only updates `name` and `avatar_url`; app update only updates name/source URL/branch/Dockerfile/replicas; tenant settings only update name/metadata. Unknown JSON fields are currently ignored by Go's decoder, but they are not assigned to persisted models.

## Improvement Opportunity
- Consider a shared strict JSON decoder with `DisallowUnknownFields` for mutation endpoints where client compatibility allows it. This would make accidental field drift fail closed instead of silently ignoring unexpected request properties.

## Positive Security Patterns Observed
- Strongly typed request structs are used throughout.
- Handlers perform explicit field copies rather than binding full request bodies directly to database models.
- Sensitive role/tenant fields are derived from auth claims or store lookups, not request bodies, in reviewed paths.
