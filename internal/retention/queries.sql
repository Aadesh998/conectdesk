-- name: get-policies
-- tenant: all, the retention worker reads every tenant's policy and applies each under its own tenant_id.
SELECT d.tenant_id, (d.value #>> '{}')::INT AS days,
    COALESCE(c.value = 'true'::jsonb, false) AS delete_contacts
FROM settings d
LEFT JOIN settings c ON c.tenant_id = d.tenant_id AND c.key = 'privacy.retention_delete_contacts'
WHERE d.key = 'privacy.retention_days' AND jsonb_typeof(d.value) = 'number' AND (d.value #>> '{}')::INT > 0
ORDER BY d.tenant_id;

-- name: delete-conversations
-- Deletes up to $3 of the tenant's resolved conversations untouched for $2
-- days. Their messages, notes, CSAT and SLA records go with them, and the
-- media cleaner removes their files.
WITH expired AS (
    SELECT c.id
    FROM conversations c
    JOIN conversation_statuses s ON s.tenant_id = c.tenant_id AND s.id = c.status_id
    WHERE c.tenant_id = $1 AND s.category = 'resolved'
      AND GREATEST(c.created_at, c.last_message_at, c.last_interaction_at, c.resolved_at, c.closed_at) < NOW() - make_interval(days => $2)
    LIMIT $3
)
DELETE FROM conversations c USING expired WHERE c.tenant_id = $1 AND c.id = expired.id;

-- name: delete-contacts
-- Deletes up to $3 of the tenant's contacts left without conversations and
-- untouched for $2 days. Blocked contacts are kept, so they stay blocked.
WITH expired AS (
    SELECT u.id
    FROM users u
    WHERE u.tenant_id = $1 AND u.type IN ('contact', 'visitor') AND u.enabled
      AND GREATEST(u.created_at, u.updated_at, u.last_active_at) < NOW() - make_interval(days => $2)
      AND NOT EXISTS (SELECT 1 FROM conversations c WHERE c.tenant_id = $1 AND c.contact_id = u.id)
    LIMIT $3
)
DELETE FROM users u USING expired WHERE u.tenant_id = $1 AND u.id = expired.id
RETURNING COALESCE(u.avatar_url, '') AS avatar_url;
