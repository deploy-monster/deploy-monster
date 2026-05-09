-- Revert built-in RBAC role permissions to the pre-0005 set.

UPDATE roles
SET permissions_json = '["tenant.*","app.*","project.*","member.*","billing.*","secret.*","server.*","domain.*","db.*"]'
WHERE id = 'role_owner' AND is_builtin = 1;

UPDATE roles
SET permissions_json = '["app.*","project.*","member.*","secret.*","server.*","billing.*","domain.*","db.*"]'
WHERE id = 'role_admin' AND is_builtin = 1;

UPDATE roles
SET permissions_json = '["app.*","project.view","secret.app.*","domain.*","db.*"]'
WHERE id = 'role_developer' AND is_builtin = 1;
