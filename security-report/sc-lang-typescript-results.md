# sc-lang-typescript Results

## Summary
TypeScript/React frontend security scan of the DeployMonster web UI.

## Findings

### Finding: TS-001
- **Title:** pnpm Override for Non-Existent lodash Version
- **Severity:** Low
- **Confidence:** 80
- **File:** web/package.json:57
- **Description:** `"lodash@4": "^4.18.0"` override targets a version that does not exist (latest lodash v4 is 4.17.21). This may cause resolution confusion.
- **Remediation:** Remove stale pnpm overrides.

### Finding: TS-002
- **Title:** Unmaintained dagre Dependency
- **Severity:** Low
- **Confidence:** 90
- **File:** web/package.json:22
- **Description:** `dagre` 0.8.5 was last published in 2017 and is unmaintained.
- **Remediation:** Evaluate `@dagrejs/dagre` community fork.

## Positive Security Patterns Observed
- React 19 with automatic XSS protection via JSX escaping
- TypeScript strict mode (implied by build configuration)
- Vite build with modern security defaults
- No `dangerouslySetInnerHTML` usage detected in source
- No `eval()` or `Function()` usage detected
- ESLint configured with react-hooks and react-refresh plugins
