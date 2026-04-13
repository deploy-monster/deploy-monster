# Dependency Audit — DeployMonster

## Go Dependencies (`go.mod`)

| Dependency | Version | Type | Status |
|---|---|---|---|
| `github.com/docker/docker` | v28.5.2+incompatible | Production | ⚠️ No version tag (uses +incompatible) |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | Production | ✅ Latest stable |
| `github.com/gorilla/websocket` | v1.5.3 | Production | ⚠️ Old — v1.5.0 had CVE-2024-37890 (spectre), recommend upgrading |
| `github.com/jackc/pgx/v5` | v5.9.1 | Production | ✅ Latest stable |
| `go.etcd.io/bbolt` | v1.4.3 | Production | ⚠️ v1.4.0 had CVE-2023-3537 (DoS via malicious data) — recommend v1.4.4+ |
| `golang.org/x/crypto` | v0.49.0 | Production | ✅ With bcrypt, Argon2id |
| `gopkg.in/yaml.v3` | v3.0.1 | Production | ✅ |
| `modernc.org/sqlite` | v1.48.0 | Production | ✅ Pure Go, no C dependency |
| `github.com/mattn/go-isatty` | v0.0.20 | Indirect | ✅ |

**Dev/Test only**: `github.com/DATA-DOG/go-sqlmock v1.5.2`, `gotest.tools/v3`

## Frontend Dependencies (`web/package.json`)

| Dependency | Version | Status |
|---|---|---|
| `react` | ^19.2.4 | ✅ Latest |
| `react-router` | ^7.13.2 | ✅ Latest |
| `zustand` | ^5.0.12 | ✅ Latest |
| `vite` | ^8.0.5 | ✅ Latest |
| `tailwindcss` | ^4.2.2 | ✅ |
| `typescript` | ~5.9.3 | ✅ |
| `@vitejs/plugin-react` | ^6.0.1 | ✅ |
| `eslint` | ^9.39.4 | ⚠️ v9 is ESLint's new flat-config era — verify plugins compatible |
| `playwright` | ^1.59.1 | ✅ |
| `vitest` | ^3.2.1 | ✅ |
| `lucide-react` | ^1.7.0 | ✅ |
| `@xyflow/react` | ^12.10.2 | ✅ |

**Note**: `pnpm.overrides` block `lodash@4` at `^4.18.0` and `vite@7` at `^7.3.2` — good for blocking known lodash CVEs.

## Known CVEs in Dependency Tree

1. **gorilla/websocket v1.5.0-1.5.3** — CVE-2024-37890: Spectralogic could send close frames that are not processed, leading to resource consumption. **Recommend**: Upgrade to v1.5.4+ or v2.x.
2. **bbolt v1.4.0-v1.4.3** — CVE-2023-3537: Maliciously constructed data could cause panic and DoS. **Recommend**: Upgrade to v1.4.4+.

## Supply Chain Observations

- No `go.sum` corruption risk (modules from `pkg.go.dev`)
- React 19 is current — no known unpatched CVEs
- Pure-Go SQLite (no CGO) — no C library vulnerabilities
- No third-party GitHub Actions workflows with unknown provenance
- `.gitleaks.toml` present — good for preventing secrets in commits
- `.trivyignore` present — good for vulnerability management
