-- name: upsert-admin
-- tenant: all, platform admins run the installation and belong to no tenant.
INSERT INTO platform_admins (email, password_hash) VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = NOW()
RETURNING id, created_at, email, password_hash, last_login_at, (totp_enabled_at IS NOT NULL) AS totp_enabled;

-- name: get-admin-by-email
-- tenant: all, platform admins run the installation and belong to no tenant.
SELECT id, created_at, email, password_hash, last_login_at, (totp_enabled_at IS NOT NULL) AS totp_enabled FROM platform_admins WHERE email = $1;

-- name: get-admins
-- tenant: all, platform admins run the installation and belong to no tenant.
SELECT id, created_at, email, password_hash, last_login_at, (totp_enabled_at IS NOT NULL) AS totp_enabled FROM platform_admins ORDER BY id;

-- name: delete-admin
-- tenant: all, platform admins run the installation and belong to no tenant.
DELETE FROM platform_admins WHERE email = $1;

-- name: touch-admin-login
-- tenant: all, platform admins run the installation and belong to no tenant.
UPDATE platform_admins SET last_login_at = NOW() WHERE id = $1;

-- name: insert-session
-- tenant: all, platform sessions belong to platform admins.
INSERT INTO platform_sessions (admin_id, token_hash, expires_at, ip) VALUES ($1, $2, $3, $4);

-- name: get-session-admin
-- tenant: all, platform sessions belong to platform admins.
SELECT a.id, a.created_at, a.email, a.password_hash, a.last_login_at, (a.totp_enabled_at IS NOT NULL) AS totp_enabled
FROM platform_sessions s JOIN platform_admins a ON a.id = s.admin_id
WHERE s.token_hash = $1 AND s.expires_at > NOW();

-- name: delete-session
-- tenant: all, platform sessions belong to platform admins.
DELETE FROM platform_sessions WHERE token_hash = $1;

-- name: delete-expired-sessions
-- tenant: all, platform sessions belong to platform admins.
DELETE FROM platform_sessions WHERE expires_at < NOW();

-- name: delete-expired-impersonations
-- tenant: all, impersonation links are created by platform admins for any workspace.
DELETE FROM platform_impersonations WHERE expires_at < NOW() - INTERVAL '1 day';

-- name: insert-audit
-- tenant: all, the platform audit log records actions on any workspace.
INSERT INTO platform_audit_log (admin_id, admin_email, action, workspace_id, workspace_slug, details, ip)
VALUES (NULLIF($1, 0), $2, $3, NULLIF($4, 0), $5, $6, $7);

-- name: get-audit
-- tenant: all, the platform audit log records actions on any workspace.
SELECT id, created_at, admin_email, action, COALESCE(workspace_id, 0) AS workspace_id, workspace_slug, details, ip
FROM platform_audit_log
WHERE ($1 = 0 OR workspace_id = $1)
ORDER BY id DESC
LIMIT $2;

-- name: get-overview
-- tenant: all, the platform overview counts across every workspace.
SELECT
    (SELECT COUNT(*) FROM tenants) AS workspaces,
    (SELECT COUNT(*) FROM tenants WHERE status = 'active') AS active,
    (SELECT COUNT(*) FROM tenants WHERE status = 'suspended') AS suspended,
    (SELECT COUNT(*) FROM tenants WHERE created_at > NOW() - INTERVAL '7 days') AS signups_7d,
    (SELECT COUNT(*) FROM tenants WHERE created_at > NOW() - INTERVAL '30 days') AS signups_30d,
    (SELECT COUNT(DISTINCT tenant_id) FROM users WHERE type = 'agent' AND last_active_at > NOW() - INTERVAL '7 days') AS active_workspaces_7d,
    (SELECT COUNT(*) FROM users WHERE type = 'agent' AND deleted_at IS NULL AND email <> 'System') AS agents,
    (SELECT COUNT(*) FROM conversations WHERE created_at > NOW() - INTERVAL '30 days') AS conversations_30d;

-- name: get-plan-counts
-- tenant: all, the platform overview counts across every workspace.
SELECT "plan", COUNT(*) AS count FROM tenants GROUP BY "plan" ORDER BY count DESC, "plan";

-- name: get-daily-signups
-- tenant: all, the platform overview counts across every workspace.
SELECT d::date AS day, COUNT(t.id) AS count
FROM generate_series(CURRENT_DATE - 29, CURRENT_DATE, INTERVAL '1 day') d
LEFT JOIN tenants t ON t.created_at::date = d::date
GROUP BY d ORDER BY d;

-- name: get-workspaces
-- tenant: all, the platform lists every workspace.
-- $1 searches slug, name, custom domain and agent emails; $2 filters by status.
SELECT t.id, t.slug, t."name",
    CASE WHEN t.deletion_scheduled_at IS NOT NULL THEN 'deleting' ELSE t.status::text END AS status,
    t."plan", t.created_at, t.custom_domain,
    (SELECT COUNT(*) FROM users u WHERE u.tenant_id = t.id AND u.type = 'agent' AND u.deleted_at IS NULL AND u.email <> 'System') AS agents,
    (SELECT COUNT(*) FROM inboxes i WHERE i.tenant_id = t.id AND i.deleted_at IS NULL) AS inboxes,
    (SELECT COUNT(*) FROM conversations c WHERE c.tenant_id = t.id AND c.created_at > NOW() - INTERVAL '30 days') AS conversations_30d,
    (SELECT MAX(u.last_active_at) FROM users u WHERE u.tenant_id = t.id AND u.type = 'agent') AS last_active_at,
    COUNT(*) OVER () AS total
FROM tenants t
WHERE ($1 = '' OR t.slug ILIKE '%' || $1 || '%' OR t."name" ILIKE '%' || $1 || '%' OR t.custom_domain ILIKE '%' || $1 || '%'
       OR EXISTS (SELECT 1 FROM users u WHERE u.tenant_id = t.id AND u.type = 'agent' AND u.email ILIKE '%' || $1 || '%'))
  AND ($2 = '' OR t.status::text = $2)
ORDER BY t.id DESC
LIMIT $3 OFFSET $4;

-- name: get-workspace-counts
-- tenant: all, the platform reads any workspace; $1 is the workspace.
SELECT
    (SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND type = 'contact' AND deleted_at IS NULL) AS contacts,
    (SELECT COUNT(*) FROM conversations WHERE tenant_id = $1) AS conversations,
    (SELECT COUNT(*) FROM conversations WHERE tenant_id = $1 AND created_at > NOW() - INTERVAL '30 days') AS conversations_30d,
    (SELECT COUNT(*) FROM help_centers WHERE tenant_id = $1) AS help_centers;

-- name: get-workspace-agents
-- tenant: all, the platform reads any workspace; $1 is the workspace.
SELECT u.id, u.first_name, COALESCE(u.last_name, '') AS last_name, u.email, u.enabled, u.last_active_at, u.last_login_at,
    ARRAY(SELECT r."name" FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id ORDER BY r."name") AS roles
FROM users u
WHERE u.tenant_id = $1 AND u.type = 'agent' AND u.deleted_at IS NULL AND u.email <> 'System'
ORDER BY u.id;

-- name: insert-impersonation
-- tenant: all, impersonation links are created by platform admins for any workspace.
INSERT INTO platform_impersonations (admin_id, workspace_id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: consume-impersonation
-- tenant: all, impersonation links are created by platform admins for any workspace.
-- Uses the link up, so it works once, and only on its own workspace.
UPDATE platform_impersonations i SET used_at = NOW()
FROM platform_admins a
WHERE i.token_hash = $1 AND i.workspace_id = $2 AND i.used_at IS NULL AND i.expires_at > NOW() AND a.id = i.admin_id
RETURNING i.admin_id, a.email AS admin_email, i.user_id;

-- name: get-admin-totp
-- tenant: all, platform admins run the installation and belong to no tenant.
SELECT COALESCE(totp_secret, '') AS secret, totp_enabled_at AS enabled_at, cardinality(totp_recovery_codes) AS recovery_codes_left
FROM platform_admins WHERE id = $1;

-- name: enable-admin-totp
-- tenant: all, platform admins run the installation and belong to no tenant.
UPDATE platform_admins
SET totp_secret = $2, totp_enabled_at = NOW(), totp_recovery_codes = $3, totp_last_step = $4, updated_at = NOW()
WHERE id = $1;

-- name: reset-admin-totp
-- tenant: all, platform admins run the installation and belong to no tenant.
UPDATE platform_admins
SET totp_secret = NULL, totp_enabled_at = NULL, totp_recovery_codes = '{}', totp_last_step = 0, updated_at = NOW()
WHERE email = $1;

-- name: use-admin-totp-step
-- tenant: all, platform admins run the installation and belong to no tenant.
UPDATE platform_admins SET totp_last_step = $2
WHERE id = $1 AND totp_enabled_at IS NOT NULL AND totp_last_step < $2
RETURNING id;

-- name: use-admin-recovery-code
-- tenant: all, platform admins run the installation and belong to no tenant.
UPDATE platform_admins SET totp_recovery_codes = array_remove(totp_recovery_codes, $2)
WHERE id = $1 AND totp_enabled_at IS NOT NULL AND $2 = ANY(totp_recovery_codes)
RETURNING cardinality(totp_recovery_codes);

-- name: get-platform-setting
-- tenant: all, platform settings belong to the installation.
SELECT value FROM platform_settings WHERE key = $1;

-- name: set-platform-setting
-- tenant: all, platform settings belong to the installation.
INSERT INTO platform_settings (key, value, updated_at) VALUES ($1, $2, NOW())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
