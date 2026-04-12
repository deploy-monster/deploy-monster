-- DeployMonster rollback — PostgreSQL dialect
-- Version: 0001 (down)
-- Order: reverse dependency order (children before parents)
-- Mirrors 0001_init.down.sql; identical semantics with Postgres syntax.

DROP INDEX IF EXISTS idx_audit_tenant;
DROP INDEX IF EXISTS idx_usage_tenant_hour;

DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS marketplace_installs;
DROP TABLE IF EXISTS compose_stacks;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS vps_providers;
DROP TABLE IF EXISTS backups;
DROP TABLE IF EXISTS volumes;
DROP TABLE IF EXISTS managed_dbs;
DROP TABLE IF EXISTS webhook_logs;
DROP TABLE IF EXISTS webhooks;
DROP TABLE IF EXISTS git_sources;
DROP TABLE IF EXISTS secret_versions;
DROP TABLE IF EXISTS secrets;
DROP TABLE IF EXISTS servers;
DROP TABLE IF EXISTS ssl_certs;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS applications;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
