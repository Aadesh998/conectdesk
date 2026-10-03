-- name: get-tenant
-- tenant: all, this is the tenant registry itself.
SELECT id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at
FROM tenants
WHERE id = $1;

-- name: get-tenant-by-slug
-- tenant: all, this is the tenant registry itself.
SELECT id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at
FROM tenants
WHERE slug = $1;

-- name: get-tenant-by-domain
-- tenant: all, this is the tenant registry itself.
-- Matches a verified custom domain that is the tenant's own domain or the domain
-- of one of its active help centers. Help center domains are stored as URLs.
SELECT t.id, t.created_at, t.updated_at, t.slug, t."name", t.custom_domain, t.status, t."plan", t.meta, t.deletion_scheduled_at
FROM custom_domains d
JOIN tenants t ON t.id = d.tenant_id
WHERE d.hostname = $1 AND d.verified_at IS NOT NULL
  AND (
    LOWER(t.custom_domain) = $1
    OR EXISTS (
        SELECT 1 FROM help_centers hc
        WHERE hc.tenant_id = t.id AND hc.is_active AND hc.custom_domain <> ''
          AND LOWER(SUBSTRING(hc.custom_domain FROM '^(?:[a-zA-Z][a-zA-Z0-9+.-]*://)?([^/:?#]+)')) = $1
    )
  );

-- name: get-all-tenants
-- tenant: all, this is the tenant registry itself.
SELECT id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at
FROM tenants
ORDER BY id;

-- name: insert-tenant
-- tenant: all, this is the tenant registry itself.
INSERT INTO tenants (slug, "name", "plan")
VALUES ($1, $2, $3)
RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at;

-- name: update-tenant-status
-- tenant: all, this is the tenant registry itself.
UPDATE tenants SET status = $2::tenant_status, updated_at = NOW()
WHERE id = $1
RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at;

-- name: update-tenant-plan
-- tenant: all, this is the tenant registry itself.
UPDATE tenants SET "plan" = $2, updated_at = NOW()
WHERE id = $1
RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at;

-- name: get-tenant-usage
-- Counts what plan limits cap: active agents (the system user and AI assistants
-- don't count) and inboxes.
SELECT
    (SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND type = 'agent' AND deleted_at IS NULL AND email <> 'System') AS agents,
    (SELECT COUNT(*) FROM inboxes WHERE tenant_id = $1 AND deleted_at IS NULL) AS inboxes;

-- name: get-onboarding
-- The setup checklist: what a new workspace has done. A teammate is a second
-- active agent; the chat widget is installed once a live chat conversation came
-- in; a reply is a message an agent sent a contact.
SELECT
    EXISTS (SELECT 1 FROM inboxes WHERE tenant_id = $1 AND deleted_at IS NULL) AS inbox,
    (SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND type = 'agent' AND deleted_at IS NULL AND email <> 'System') > 1 AS teammate,
    EXISTS (SELECT 1 FROM conversations c JOIN inboxes i ON i.tenant_id = c.tenant_id AND i.id = c.inbox_id
            WHERE c.tenant_id = $1 AND i.channel = 'livechat') AS widget,
    EXISTS (SELECT 1 FROM help_articles WHERE tenant_id = $1 AND status = 'published') AS article,
    EXISTS (SELECT 1 FROM conversation_messages m JOIN users u ON u.tenant_id = m.tenant_id AND u.id = m.sender_id
            WHERE m.tenant_id = $1 AND m.type = 'outgoing' AND NOT m.private AND u.type = 'agent' AND u.email <> 'System') AS reply;

-- name: delete-tenant
-- tenant: all, this is the tenant registry itself; every tenant row cascades away with it.
DELETE FROM tenants WHERE id = $1 AND id <> 1;

-- name: get-domains
-- Help center domains are stored as URLs; used_by lists the help centers on the hostname.
SELECT d.id, d.created_at, d.updated_at, d.hostname, d.token, d.verified_at, d.checked_at, d.last_error, d.failed_checks,
    COALESCE(LOWER(t.custom_domain) = d.hostname, false) AS is_primary,
    ARRAY(
        SELECT hc."name" FROM help_centers hc
        WHERE hc.tenant_id = d.tenant_id AND hc.custom_domain <> ''
          AND LOWER(SUBSTRING(hc.custom_domain FROM '^(?:[a-zA-Z][a-zA-Z0-9+.-]*://)?([^/:?#]+)')) = d.hostname
        ORDER BY hc.id
    ) AS help_centers
FROM custom_domains d
JOIN tenants t ON t.id = d.tenant_id
WHERE d.tenant_id = $1 AND ($2 = 0 OR d.id = $2)
ORDER BY d.id;

-- name: insert-domain
INSERT INTO custom_domains (tenant_id, hostname, token)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, hostname) DO NOTHING
RETURNING id;

-- name: domain-verified-elsewhere
SELECT EXISTS (SELECT 1 FROM custom_domains WHERE tenant_id <> $1 AND hostname = $2 AND verified_at IS NOT NULL);

-- name: record-domain-check
-- Marks the domain verified when $3 is empty, else records why the check failed.
UPDATE custom_domains
SET checked_at = NOW(), updated_at = NOW(), last_error = $3,
    verified_at = CASE WHEN $3 = '' THEN COALESCE(verified_at, NOW()) ELSE verified_at END
WHERE tenant_id = $1 AND id = $2;

-- name: delete-domain
DELETE FROM custom_domains WHERE tenant_id = $1 AND id = $2;

-- name: set-primary-domain
-- tenant: all, this is the tenant registry itself; $1 is the tenant's own id.
-- NULL makes the tenant's subdomain its address again.
UPDATE tenants SET custom_domain = $2, updated_at = NOW() WHERE id = $1;

-- name: verified-domain-exists
-- tenant: all, answers whether any tenant holds the hostname verified, for TLS issuance.
SELECT EXISTS (SELECT 1 FROM custom_domains WHERE hostname = $1 AND verified_at IS NOT NULL);

-- name: trust-domain
-- Records the hostname as verified without a DNS check, for single-tenant installations.
INSERT INTO custom_domains (tenant_id, hostname, token, verified_at, checked_at, trusted)
VALUES ($1, $2, $3, NOW(), NOW(), true)
ON CONFLICT (tenant_id, hostname) DO UPDATE
SET verified_at = COALESCE(custom_domains.verified_at, NOW()), trusted = true, updated_at = NOW();

-- name: schedule-tenant-deletion
-- tenant: all, this is the tenant registry itself. The default tenant can't be deleted.
UPDATE tenants SET status = 'suspended', deletion_scheduled_at = $2, updated_at = NOW()
WHERE id = $1 AND id <> 1
RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at;

-- name: cancel-tenant-deletion
-- tenant: all, this is the tenant registry itself.
UPDATE tenants SET status = 'active', deletion_scheduled_at = NULL, updated_at = NOW()
WHERE id = $1 AND deletion_scheduled_at IS NOT NULL
RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at;

-- name: get-tenants-due-for-deletion
-- tenant: all, the deletion worker deletes every tenant whose grace period ended.
SELECT id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at
FROM tenants WHERE deletion_scheduled_at <= NOW() AND id <> 1 ORDER BY id;

-- name: get-tenant-media-names
-- Names of the tenant's stored files, deleted from the store with the tenant.
SELECT "uuid"::text FROM media WHERE tenant_id = $1;

-- name: get-domains-to-recheck
-- tenant: all, the re-check worker looks at every tenant's verified domains.
SELECT d.tenant_id, d.id, d.hostname, d.token, d.failed_checks
FROM custom_domains d
WHERE d.verified_at IS NOT NULL AND NOT d.trusted
ORDER BY d.tenant_id, d.id;

-- name: record-domain-recheck
-- A passing check ($3 empty) clears failures; a failing one counts, and at $4
-- failures in a row unverifies the domain.
UPDATE custom_domains
SET checked_at = NOW(), updated_at = NOW(), last_error = $3,
    failed_checks = CASE WHEN $3 = '' THEN 0 ELSE failed_checks + 1 END,
    verified_at = CASE WHEN $3 <> '' AND failed_checks + 1 >= $4 THEN NULL ELSE verified_at END
WHERE tenant_id = $1 AND id = $2
RETURNING verified_at IS NULL AS unverified;

-- name: clear-primary-domain-if
-- tenant: all, this is the tenant registry itself; $1 is the tenant's own id.
UPDATE tenants SET custom_domain = NULL, updated_at = NOW() WHERE id = $1 AND LOWER(custom_domain) = $2;
