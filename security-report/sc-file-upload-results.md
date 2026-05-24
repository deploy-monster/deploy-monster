# sc-file-upload Results

## Summary
File upload and import security scan.

## Findings

No active arbitrary file-upload or archive-extraction findings are verified in the current working tree.

## Resolved / Revalidated Items

### FU-001: Certificate Upload Without File Type Validation
- **Previous Severity:** Medium
- **Status:** RESOLVED / NOT A RAW FILE UPLOAD
- **File:** `internal/api/handlers/certificates.go`
- **Notes:** `POST /api/v1/certificates` accepts JSON PEM fields, not multipart file uploads. The handler parses the certificate/key pair with `tls.X509KeyPair`, parses the leaf certificate with `x509.ParseCertificate`, requires a tenant-owned domain, and validates that SAN/CN matches that domain before storage.

### FU-002: App Import Accepts Arbitrary Archives
- **Previous Severity:** Medium
- **Status:** NOT APPLICABLE / HARDENED
- **File:** `internal/api/handlers/import_export.go`
- **Notes:** App import accepts a JSON `AppManifest`; it does not extract archives, so zip slip/path traversal through archive members is not present. Manifest `source_url` validation now reuses the same URL policy as normal app create/update.

## Positive Security Patterns Observed
- Global body limit of 10 MB restricts upload size.
- Webhook body limit of 1 MB.
- Backup downloads require authentication and tenant-scoped key validation.
- Certificate uploads require parsable PEM cert/key material and tenant-domain matching.
