# sc-mass-assignment Results

## Summary
Mass assignment security scan.

## Findings

### Finding: MA-001
- **Title:** PATCH Handlers May Allow Unintended Field Updates
- **Severity:** Medium
- **Confidence:** 60
- **File:** internal/api/router.go:140
- **Description:** `PATCH /api/v1/auth/me` and similar PATCH endpoints may allow mass assignment if the update handler maps the entire request body to the model without field whitelisting.
- **Remediation:** Implement explicit field whitelisting for all PATCH/PUT handlers. Reject unknown fields.

## Positive Security Patterns Observed
- Strongly typed request structs used throughout
- JSON unmarshaling into known structs limits arbitrary field injection
- Some handlers use explicit DTOs
