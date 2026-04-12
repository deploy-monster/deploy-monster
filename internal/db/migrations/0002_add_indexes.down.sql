-- Revert: Remove indexes added in 0002
DROP INDEX IF EXISTS idx_applications_tenant;
DROP INDEX IF EXISTS idx_applications_project;
DROP INDEX IF EXISTS idx_deployments_app;
DROP INDEX IF EXISTS idx_domains_app;
DROP INDEX IF EXISTS idx_team_members_user;
DROP INDEX IF EXISTS idx_projects_tenant;
DROP INDEX IF EXISTS idx_secrets_tenant;
DROP INDEX IF EXISTS idx_secret_versions_secret;
DROP INDEX IF EXISTS idx_invitations_tenant;
DROP INDEX IF EXISTS idx_backups_tenant;
DROP INDEX IF EXISTS idx_webhook_logs_webhook;
