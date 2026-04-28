# sc-xxe Results

## Summary
XML external entity security scan.

## Findings

No issues found. (Not applicable: no XML parsing of user input.)

## Analysis
- API accepts JSON and YAML only
- Only XML usage is in `internal/backup/s3.go` for parsing S3 ListObjectsV2 responses from trusted AWS/MinIO APIs
- Go's `encoding/xml` does not process external entities by default
- No SOAP, SVG, XLSX, DOCX, or RSS processing found
