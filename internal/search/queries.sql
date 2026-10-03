-- name: search-conversations-by-reference-number
SELECT
    conversations.created_at,
    conversations.uuid,
    conversations.reference_number,
    conversations.subject,
    cs.name AS status
FROM conversations
LEFT JOIN conversation_statuses cs ON conversations.status_id = cs.id
WHERE conversations.tenant_id = $1 AND reference_number::text = $2;

-- name: search-conversations-by-contact-email
SELECT
    conversations.created_at,
    conversations.uuid,
    conversations.reference_number,
    conversations.subject,
    cs.name AS status
FROM conversations
JOIN users ON conversations.contact_id = users.id
LEFT JOIN conversation_statuses cs ON conversations.status_id = cs.id
WHERE conversations.tenant_id = $1 AND users.email ILIKE '%' || $2 || '%'
ORDER BY conversations.created_at DESC
LIMIT 1000;

-- name: search-messages
SELECT
    c.created_at as "conversation_created_at",
    c.reference_number as "conversation_reference_number",
    c.uuid as "conversation_uuid",
    LEFT(m.text_content, 200) AS text_content,
    cs.name as "conversation_status"
FROM conversation_messages m
    JOIN conversations c ON m.conversation_id = c.id
    LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
WHERE m.tenant_id = $1 AND m.type != 'activity' and m.text_content ILIKE '%' || $2 || '%'
LIMIT 30;

-- name: search-contacts
SELECT
    id,
    created_at,
    first_name,
    last_name,
    email,
    external_user_id
FROM users
WHERE tenant_id = $1
AND type = 'contact'
AND deleted_at IS NULL
AND email ILIKE '%' || $2 || '%'
LIMIT 15;
