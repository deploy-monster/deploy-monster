-- Add indexes for hot query patterns missed by 0002
-- Version: 0003
-- Dialect: SQLite
--
-- Three lookup patterns were not covered by 0002's single-column
-- foreign-key indexes and were the top hot-path gaps found in the
-- Sprint-3 DB-index audit:
--
--   1. secrets(scope, name) — GetSecretByScopeAndName runs on every
--      secret resolution during a deploy and during every container
--      config refresh. Without this composite, the lookup falls back
--      to a full table scan.
--   2. deployments(status) — ListDeploymentsByStatus runs at master
--      boot to reclaim in-flight deployments (RACE-002 recovery path).
--      Low frequency but against a table that grows with every
--      deploy; a full scan eventually dominates cold-start time.
--   3. applications(tenant_id, name) — GetAppByName. The existing
--      idx_applications_tenant serves list-by-tenant; this composite
--      adds the name-on-top selectivity for the unique-per-tenant
--      name lookup pattern used by the REST API and CLI.
--
-- We add these rather than replacing the existing single-column
-- indexes so a hypothetical rollback to 0002 stays trivial.

CREATE INDEX IF NOT EXISTS idx_secrets_scope_name ON secrets(scope, name);

CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);

CREATE INDEX IF NOT EXISTS idx_applications_tenant_name ON applications(tenant_id, name);
