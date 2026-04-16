-- Rollback 0003: drop the three hot-path indexes added in up.

DROP INDEX IF EXISTS idx_applications_tenant_name;
DROP INDEX IF EXISTS idx_deployments_status;
DROP INDEX IF EXISTS idx_secrets_scope_name;
