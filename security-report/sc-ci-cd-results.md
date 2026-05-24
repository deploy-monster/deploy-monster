# sc-ci-cd Results

## Summary
CI/CD security scan for the current working tree.

## Findings

No open CI/CD security findings were verified.

## Resolved Findings

### CICD-001 — GitHub Actions Workflow Permissions Not Explicitly Restricted
- **Previous severity:** Low
- **Status:** Resolved
- **Evidence:** All workflow files define explicit top-level `permissions:` blocks.

### CICD-002 — Floating GitHub Action Tags
- **Previous severity:** Low
- **Status:** Resolved
- **Evidence:** Workflow `uses:` references are pinned to full commit SHAs. Trivy was already SHA-pinned; checkout/setup/upload/pnpm/docker/goreleaser/syft actions are now pinned as well.

## Positive Security Patterns Observed
- No `pull_request_target` workflows.
- Release workflow scopes token permissions explicitly.
- Staging smoke credentials are read only from GitHub Secrets.
- All `uses:` references in `.github/workflows/*.yml` are pinned to 40-character commit SHAs.
- Gitleaks is installed as a fixed release archive and verified with SHA256 before execution.
- Local validation command `rg -n "uses:\s*[^@\s]+@" .github/workflows -g '*.yml' | awk '$0 !~ /@[0-9a-f]{40}/ { print }'` returned no unpinned action references.
