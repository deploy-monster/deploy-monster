-- Expand built-in RBAC role permissions for non-app resource domains.
-- Version: 0005
-- Dialect: SQLite

UPDATE roles
SET permissions_json = '["tenant.*","app.*","project.*","member.*","billing.*","secret.*","server.*","domain.*","db.*","network.*","volume.*","registry.*","backup.*","git.*","marketplace.*","topology.*","webhook.*","deploy.*"]'
WHERE id = 'role_owner' AND is_builtin = 1;

UPDATE roles
SET permissions_json = '["app.*","project.*","member.*","secret.*","server.*","billing.*","domain.*","db.*","network.*","volume.*","registry.*","backup.*","git.*","marketplace.*","topology.*","webhook.*","deploy.*"]'
WHERE id = 'role_admin' AND is_builtin = 1;

UPDATE roles
SET permissions_json = '["app.*","project.view","secret.app.*","domain.*","db.*","network.manage","volume.manage","registry.manage","backup.create","backup.restore","git.manage","marketplace.deploy","topology.manage","topology.deploy","webhook.manage"]'
WHERE id = 'role_developer' AND is_builtin = 1;
