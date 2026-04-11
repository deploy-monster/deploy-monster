-- Add indexes on high-query foreign key columns — PostgreSQL dialect
-- Version: 0002
-- Mirrors 0002_add_indexes.sql. Postgres supports CREATE INDEX IF NOT EXISTS
-- natively (9.5+), so the syntax here is identical to the SQLite version.

-- Applications: filtered by tenant on every list/count query
CREATE INDEX IF NOT EXISTS idx_applications_tenant ON applications(tenant_id);

-- Applications: filtered by project on project detail pages
CREATE INDEX IF NOT EXISTS idx_applications_project ON applications(project_id);

-- Deployments: filtered by app_id on every deployment list, latest lookup, max version
CREATE INDEX IF NOT EXISTS idx_deployments_app ON deployments(app_id, version DESC);

-- Domains: filtered by app_id for domain listing and bulk delete
CREATE INDEX IF NOT EXISTS idx_domains_app ON domains(app_id);

-- Team Members: filtered by user_id for membership lookup (login flow)
CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);

-- Projects: filtered by tenant_id on every project list
CREATE INDEX IF NOT EXISTS idx_projects_tenant ON projects(tenant_id);

-- Secrets: filtered by tenant_id on secret listing
CREATE INDEX IF NOT EXISTS idx_secrets_tenant ON secrets(tenant_id);

-- Secret Versions: filtered by secret_id for latest version lookup
CREATE INDEX IF NOT EXISTS idx_secret_versions_secret ON secret_versions(secret_id, version DESC);

-- Invitations: filtered by tenant_id on invite listing
CREATE INDEX IF NOT EXISTS idx_invitations_tenant ON invitations(tenant_id);

-- Backups: filtered by tenant_id on backup listing
CREATE INDEX IF NOT EXISTS idx_backups_tenant ON backups(tenant_id);

-- Webhook Logs: filtered by webhook_id for log retrieval
CREATE INDEX IF NOT EXISTS idx_webhook_logs_webhook ON webhook_logs(webhook_id);
