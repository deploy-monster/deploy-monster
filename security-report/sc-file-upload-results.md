# sc-file-upload Results

## Summary
File upload security scan.

## Findings

### Finding: FU-001
- **Title:** Certificate Upload Without File Type Validation
- **Severity:** Medium
- **Confidence:** 65
- **File:** internal/api/router.go:431-432
- **Description:** `POST /api/v1/certificates` accepts certificate uploads. If file type and content validation are insufficient, attackers could upload non-certificate files.
- **Remediation:** Validate MIME type, file extension, and parse the certificate to ensure it's a valid PEM/DER before storage.

### Finding: FU-002
- **Title:** App Import Accepts Arbitrary Archives
- **Severity:** Medium
- **Confidence:** 60
- **File:** internal/api/router.go:165
- **Description:** `POST /api/v1/apps/import` imports app definitions. If the import handler accepts archive files, zip slip or path traversal vulnerabilities may exist.
- **Remediation:** Validate extracted paths, reject `..` components, and enforce a whitelist of allowed file types.

## Positive Security Patterns Observed
- Global body limit of 10MB restricts upload size
- Webhook body limit of 1MB
- Backup downloads require authentication
