-- name: get-active-inboxes
-- tenant: all, inbox receivers of every tenant start at boot; each inbox carries its tenant_id.
SELECT id, tenant_id, uuid, created_at, updated_at, "name", deleted_at, channel, enabled, csat_enabled, prompt_tags_on_reply, config, "from", from_name_template, linked_email_inbox_id FROM inboxes where enabled is TRUE and deleted_at is NULL;

-- name: get-all-inboxes
SELECT id, tenant_id, uuid, created_at, updated_at, "name", deleted_at, channel, enabled, csat_enabled, prompt_tags_on_reply, config, "from", from_name_template, linked_email_inbox_id FROM inboxes where tenant_id = $1 and deleted_at is NULL;

-- name: insert-inbox
INSERT INTO inboxes
(tenant_id, channel, config, "name", "from", enabled, csat_enabled, prompt_tags_on_reply, secret, linked_email_inbox_id, from_name_template)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *

-- name: get-inbox
SELECT id, tenant_id, uuid, created_at, updated_at, "name", deleted_at, channel, enabled, csat_enabled, prompt_tags_on_reply, config, "from", from_name_template, secret, linked_email_inbox_id FROM inboxes where tenant_id = $1 and id = $2 and deleted_at is NULL;

-- name: get-inbox-by-uuid
SELECT id, tenant_id, uuid, created_at, updated_at, "name", deleted_at, channel, enabled, csat_enabled, prompt_tags_on_reply, config, "from", from_name_template, secret, linked_email_inbox_id FROM inboxes where tenant_id = $1 and uuid = $2 and deleted_at is NULL;

-- name: update
UPDATE inboxes
set channel = $3, config = $4, "name" = $5, "from" = $6, csat_enabled = $7, prompt_tags_on_reply = $8, enabled = $9, secret = $10, linked_email_inbox_id = $11, from_name_template = $12, updated_at = now()
where tenant_id = $1 and id = $2 and deleted_at is NULL
RETURNING *;

-- name: soft-delete
UPDATE inboxes set deleted_at = now(), updated_at = now(), config = '{}', enabled = false where tenant_id = $1 and id = $2 and deleted_at is NULL;

-- name: toggle
UPDATE inboxes
SET enabled = NOT enabled, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: update-config
UPDATE inboxes
SET config = $3, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: get-inbox-by-account
-- tenant: all, a Meta account can be connected to one workspace only, and webhooks name only the account.
SELECT id, tenant_id FROM inboxes
WHERE channel = $1::channels AND config->>'account_id' = $2 AND deleted_at IS NULL
ORDER BY id LIMIT 1;

-- name: get-inboxes-connected-by
-- tenant: all, Meta's deauthorize and data deletion callbacks name only the Meta user, who may have connected accounts in several workspaces.
SELECT id, tenant_id, uuid, created_at, updated_at, "name", deleted_at, channel, enabled, csat_enabled, prompt_tags_on_reply, config, "from", from_name_template, linked_email_inbox_id FROM inboxes
WHERE channel IN ('whatsapp', 'messenger', 'instagram') AND config->>'connected_by' = $1 AND deleted_at IS NULL;

-- name: disconnect-inbox
UPDATE inboxes
SET config = $3, enabled = false, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: reconnect-inbox
UPDATE inboxes
SET config = $3, enabled = true, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;
