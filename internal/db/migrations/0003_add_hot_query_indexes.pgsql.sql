-- Add indexes for hot query patterns missed by 0002 — PostgreSQL dialect
-- Version: 0003
-- Mirrors 0003_add_hot_query_indexes.sql. Same rationale, same three
-- composites. Postgres supports CREATE INDEX IF NOT EXISTS natively
-- (9.5+) so the syntax is identical to the SQLite variant.

CREATE INDEX IF NOT EXISTS idx_secrets_scope_name ON secrets(scope, name);

CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);

CREATE INDEX IF NOT EXISTS idx_applications_tenant_name ON applications(tenant_id, name);
