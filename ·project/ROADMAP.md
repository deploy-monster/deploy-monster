# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-08
> This roadmap prioritizes work needed to bring DeployMonster to production quality.

## Current State Assessment

DeployMonster has a **solid architectural foundation** with excellent test coverage (~92% Go, 251 test files) and a clean modular design. The core deployment pipeline (git -> build -> deploy -> route) works. The React UI covers all major flows.

**Key blockers for production readiness:**
- Security hardening (token management, CORS, password handling)
- ~30% of API endpoints return placeholder data
- Supporting modules (billing, VPS, DNS, swarm) need real-world testing with external services
- Error handling inconsistency makes production debugging difficult

**What's working well:**
- Architecture is excellent — clean interfaces, DI, no circular deps
- SQLite + BBolt database layer is production-solid
- Auth (JWT + API keys + RBAC) is functional
- Ingress/reverse proxy with ACME SSL is production-grade
- Build engine with 14 language detectors works
- Docker integration is complete
- Frontend is modern and well-structured

---

## Phase 1: Security Hardening (Week 1-2)

### Must-fix items blocking safe deployment

- [ ] **Fix admin password delivery** — Stop logging plaintext password to stdout. Print to stderr once, or write to a credentials file with 0600 permissions. Require password change on first login.
  - Files: `internal/auth/module.go` (firstRunSetup)
  - Effort: 2-4 hours

- [ ] **Restrict CORS default** — Change `middleware.CORS("*")` to use `server.domain` from config. Add configurable allowed origins.
  - Files: `internal/api/router.go`, `internal/api/middleware/middleware.go`
  - Effort: 2-3 hours

- [ ] **Implement refresh token revocation** — Add token blacklist in BBolt with TTL. Invalidate on logout, password change, and explicit revocation.
  - Files: `internal/auth/jwt.go`, `internal/auth/module.go`, `internal/db/bolt.go`
  - Effort: 4-6 hours

- [ ] **Add rate limiting on auth endpoints** — Token bucket per IP on `/api/v1/auth/login`, `/api/v1/auth/register`, `/api/v1/auth/refresh`. 5 attempts/minute.
  - Files: `internal/api/router.go`, `internal/api/middleware/ratelimit.go`
  - Effort: 3-4 hours

- [ ] **JWT key rotation support** — Add `kid` (key ID) to JWT header. Support multiple signing keys with graceful rotation. Old tokens still validate until expiry.
  - Files: `internal/auth/jwt.go`, `internal/core/config.go`
  - Effort: 6-8 hours

- [ ] **Move tokens to httpOnly cookies** — Replace localStorage token storage with httpOnly + Secure + SameSite cookies. Add CSRF protection for state-changing requests.
  - Files: `internal/auth/module.go`, `web/src/api/client.ts`, `web/src/stores/auth.ts`
  - Effort: 8-12 hours

---

## Phase 2: Error Handling & Observability (Week 3-4)

### Make the system debuggable in production

- [ ] **Add request ID to all error responses** — Include X-Request-ID in error JSON responses. Log request ID with every error.
  - Files: `internal/api/helpers.go`, `internal/api/middleware/middleware.go`
  - Effort: 3-4 hours

- [ ] **Fix silent error swallowing** — Audit all handlers for ignored error returns. Log every error with context, request ID, user ID.
  - Files: All handler files in `internal/api/handlers/`, module files
  - Effort: 8-12 hours

- [ ] **Structured error types** — Create `APIError` type with code, message, request ID, and optional details. Consistent error response format.
  - Files: `internal/core/errors.go`, `internal/api/helpers.go`
  - Effort: 4-6 hours

- [ ] **Prometheus metrics for API** — Request count, latency histogram, error rate, active connections. Expose on `/metrics`.
  - Files: `internal/api/middleware/`, `internal/resource/`
  - Effort: 6-8 hours

- [ ] **ACME renewal error handling** — Surface renewal failures via health check and notifications. Add renewal status to health endpoint.
  - Files: `internal/ingress/acme.go`, `internal/ingress/module.go`
  - Effort: 3-4 hours

---

## Phase 3: Core Feature Completion (Week 5-8)

### Complete the features that are currently stubs

- [ ] **Audit placeholder handlers** — Identify all 231 endpoints, mark which return real data vs placeholder. Remove or clearly mark stubs.
  - Files: All files in `internal/api/handlers/`
  - Effort: 4-6 hours (audit), 20-40 hours (implement real ones)

- [ ] **Complete Git source OAuth flows** — Test and fix GitHub/GitLab/Gitea OAuth with real credentials. Webhook auto-registration.
  - Files: `internal/gitsources/providers/`, `internal/gitsources/oauth.go`
  - Effort: 12-16 hours

- [ ] **Complete DNS Cloudflare integration** — Test with real Cloudflare API. Auto-create A records for new apps. DNS verification flow.
  - Files: `internal/dns/providers/cloudflare.go`, `internal/dns/sync.go`
  - Effort: 8-12 hours

- [ ] **Complete backup storage backends** — Test local + S3 storage with real uploads/downloads. Implement restore flow. Add encryption at rest.
  - Files: `internal/backup/storage/local.go`, `internal/backup/storage/s3.go`
  - Effort: 12-16 hours

- [ ] **Complete managed database provisioning** — Test PostgreSQL/MySQL/Redis container creation with real Docker. Connection string generation. Backup integration.
  - Files: `internal/database/engines/`, `internal/database/provisioner.go`
  - Effort: 12-16 hours

- [ ] **Complete marketplace deploy flow** — Template -> config form -> compose deploy. Test with 5+ real templates (WordPress, Gitea, n8n).
  - Files: `internal/marketplace/deployer.go`, `internal/marketplace/wizard.go`
  - Effort: 8-12 hours

---

## Phase 4: Testing & Quality (Week 9-10)

### Comprehensive test coverage

- [ ] **Frontend test coverage to 40%+** — Add tests for auth store, topology store, API client, deploy wizard, app detail page.
  - Files: `web/src/test/`, new test files for stores/pages/hooks
  - Effort: 16-24 hours

- [ ] **WebSocket handler tests** — Increase `internal/api/ws` from 66% to 85%+. Test connection lifecycle, error handling, auth.
  - Files: `internal/api/ws/*_test.go`
  - Effort: 6-8 hours

- [ ] **Deploy module to 85%+** — Currently 78.9%. Add tests for edge cases: failed deploys, concurrent deploys, rollback flows.
  - Files: `internal/deploy/*_test.go`
  - Effort: 8-12 hours

- [ ] **Integration tests with real Docker** — Test full deploy pipeline: create app -> deploy -> verify container running -> stop -> remove. Requires Docker in CI.
  - Files: New `internal/integration/` package
  - Effort: 12-16 hours

- [ ] **Add Playwright e2e tests** — Login -> create app -> deploy -> verify. Cover critical user flows.
  - Files: New `web/e2e/` directory
  - Effort: 16-24 hours

- [ ] **Run race detector in CI** — Add `-race` flag to `go test` in GitHub Actions.
  - Files: `.github/workflows/ci.yml`
  - Effort: 1 hour

---

## Phase 5: Performance & Optimization (Week 11-12)

### Performance tuning

- [ ] **BBolt write optimization** — Batch metrics writes. Use ring buffer pattern with periodic flush instead of per-event writes.
  - Files: `internal/resource/collector.go`, `internal/db/bolt.go`
  - Effort: 6-8 hours

- [ ] **Async event handler pool** — Limit concurrent async event goroutines with a worker pool. Prevent goroutine leaks under load.
  - Files: `internal/core/events.go`
  - Effort: 4-6 hours

- [ ] **HTTP response caching** — Add ETag/Last-Modified for static API responses (marketplace templates, app list). Cache control headers.
  - Files: `internal/api/middleware/`
  - Effort: 4-6 hours

- [ ] **Frontend bundle optimization** — Analyze chunks. Consider dynamic import for @xyflow/react. Add Vite bundle reporter to CI.
  - Files: `web/vite.config.ts`, CI config
  - Effort: 4-6 hours

- [ ] **Load testing** — Set up k6 or vegeta test suite. Validate: 100 concurrent users, p95 < 100ms API, 1000 req/s proxy.
  - Files: New `tests/load/` directory
  - Effort: 8-12 hours

---

## Phase 6: Documentation & DX (Week 13-14)

### Documentation and developer experience

- [ ] **Fix documentation inconsistencies** — PROJECT_STATUS.md claims RS256 JWT, code uses HS256. Align all docs with reality.
  - Files: `PROJECT_STATUS.md`, `docs/architecture.md`
  - Effort: 4-6 hours

- [ ] **API documentation with real examples** — Update OpenAPI spec. Add request/response examples for all 231 endpoints.
  - Files: `docs/openapi.yaml`, `docs/examples/api-quickstart.md`
  - Effort: 12-16 hours

- [ ] **Contributing guide update** — Development setup, architecture overview, module creation guide, testing guide.
  - Files: `CONTRIBUTING.md`
  - Effort: 4-6 hours

- [ ] **Configuration reference** — Document every config option in `monster.yaml` with defaults, env var overrides, and examples.
  - Files: New `docs/configuration.md`
  - Effort: 6-8 hours

---

## Phase 7: Release Preparation (Week 15-16)

### Final production preparation

- [ ] **CI/CD hardening** — Code coverage thresholds (fail if below 85% Go, 30% frontend). Bundle size budget. Security scanning (govulncheck).
  - Files: `.github/workflows/ci.yml`
  - Effort: 4-6 hours

- [ ] **Docker image optimization** — Verify non-root user works with all features. Health check covers critical modules. Add resource limits.
  - Files: `Dockerfile`, `docker-compose.yml`
  - Effort: 4-6 hours

- [ ] **Migration rollback support** — Add down migrations for each up migration. Test rollback path.
  - Files: `internal/db/migrations/`, `internal/db/sqlite.go`
  - Effort: 8-12 hours

- [ ] **Config path flag implementation** — `--config` flag is parsed but unused. Wire it to config loading.
  - Files: `cmd/deploymonster/main.go`, `internal/core/config.go`
  - Effort: 1-2 hours

- [ ] **Production deployment guide** — Step-by-step guide for Ubuntu/Debian, systemd service file, backup cron, monitoring setup.
  - Files: `docs/deployment-guide.md`, new `deployments/` examples
  - Effort: 6-8 hours

---

## Beyond v1.0: Future Enhancements

- [ ] PostgreSQL Store implementation (enterprise)
- [ ] Kubernetes support alongside Docker Swarm
- [ ] Multi-region deployments
- [ ] GPU workload support
- [ ] Edge deployments
- [ ] Terraform provider
- [ ] Real-time collaborative topology editing
- [ ] Plugin system for custom modules
- [ ] Mobile app (React Native)
- [ ] Webhook retry with exponential backoff
- [ ] Secret rotation automation
- [ ] Canary deployment with metrics-based auto-promote/rollback

---

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|---|---|---|---|
| Phase 1: Security | 25-37h | CRITICAL | None |
| Phase 2: Observability | 24-34h | CRITICAL | None |
| Phase 3: Features | 68-98h | HIGH | Phase 1 |
| Phase 4: Testing | 59-85h | HIGH | Phase 2, 3 |
| Phase 5: Performance | 26-38h | MEDIUM | Phase 3 |
| Phase 6: Documentation | 26-36h | MEDIUM | Phase 3 |
| Phase 7: Release | 23-34h | HIGH | Phase 1-4 |
| **Total** | **251-362h** | | |

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| External API integrations (Stripe, Hetzner, etc.) fail in production | High | High | Integration test suite with sandbox APIs; circuit breaker pattern |
| SQLite write contention under load | Medium | Medium | Connection pooling tuning; PostgreSQL migration path exists |
| Security vulnerability discovered post-launch | Medium | High | Automated govulncheck in CI; bug bounty program; rapid response process |
| Agent mode doesn't scale beyond 3-5 nodes | High | Medium | Swarm module needs real-world testing; document single-node as supported initially |
| Frontend bundle grows beyond 2MB | Low | Low | Bundle budget in CI; lazy loading already in place |
| Breaking API changes needed before v1.0 | Medium | Medium | API versioning (`/api/v2`); deprecation headers; client SDK versioning |
