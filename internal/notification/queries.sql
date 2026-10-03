-- name: get-notifications
SELECT
    n.id, n.created_at, n.updated_at, n.user_id, n.notification_type,
    n.title, n.body, n.is_read, n.conversation_id, n.message_id, n.actor_id, n.meta,
    u.first_name as actor_first_name, u.last_name as actor_last_name, u.avatar_url as actor_avatar_url,
    c.uuid as conversation_uuid, m.uuid as message_uuid
FROM user_notifications n
LEFT JOIN users u ON u.id = n.actor_id
LEFT JOIN conversations c ON c.id = n.conversation_id
LEFT JOIN conversation_messages m ON m.id = n.message_id
WHERE n.tenant_id = $1 AND n.user_id = $2
ORDER BY n.created_at DESC
LIMIT $3 OFFSET $4;

-- name: get-notification-stats
SELECT
    COUNT(*) FILTER (WHERE is_read = false) as unread_count,
    COUNT(*) as total_count
FROM user_notifications
WHERE tenant_id = $1 AND user_id = $2;

-- name: insert-notification
INSERT INTO user_notifications (tenant_id, user_id, notification_type, title, body, conversation_id, message_id, actor_id, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, created_at, updated_at, user_id, notification_type, title, body, is_read, conversation_id, message_id, actor_id, meta;

-- name: mark-as-read
UPDATE user_notifications SET is_read = true, updated_at = now() WHERE tenant_id = $1 AND id = $2 AND user_id = $3 RETURNING id;

-- name: mark-assignment-as-read
UPDATE user_notifications
SET is_read = true, updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND notification_type = 'assignment'
  AND is_read = false;

-- name: mark-all-as-read
UPDATE user_notifications SET is_read = true, updated_at = now() WHERE tenant_id = $1 AND user_id = $2 AND is_read = false;

-- name: delete-notification
DELETE FROM user_notifications WHERE tenant_id = $1 AND id = $2 AND user_id = $3;

-- name: delete-all-notifications
DELETE FROM user_notifications WHERE tenant_id = $1 AND user_id = $2;

-- name: delete-old-notifications
-- tenant: all, the retention cleaner applies to every tenant.
DELETE FROM user_notifications WHERE created_at < NOW() - INTERVAL '30 days';
