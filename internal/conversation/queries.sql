-- name: unsnooze-all
-- tenant: all, the unsnoozer covers every tenant; statuses are looked up in each conversation's tenant.
UPDATE conversations
SET snoozed_until = NULL, status_id = (SELECT id FROM conversation_statuses s WHERE s.tenant_id = conversations.tenant_id AND s.name = 'Open')
WHERE snoozed_until <= NOW()
  AND status_id = (SELECT id FROM conversation_statuses s WHERE s.tenant_id = conversations.tenant_id AND s.name = 'Snoozed');

-- name: insert-conversation
-- $1 = tenant, $12 = rate limit window start (timestamptz), $13 = max conversations (0 = unlimited)
-- $14 = subject reference marker template (placeholder: {ref})
WITH
status_id AS (
    SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND name = $3
),
reference_number AS (
    SELECT generate_reference_number($1, $8) AS reference_number
)
INSERT INTO conversations
(tenant_id, contact_id, status_id, inbox_id, last_message, last_message_at, subject, reference_number, meta, custom_attributes)
SELECT
   $1,
   $2,
   (SELECT id FROM status_id),
   $4,
   $5,
   $6,
   CASE
      WHEN $9 = TRUE THEN CONCAT($7::text, ' - ', REPLACE($14::text, '{ref}', (SELECT reference_number FROM reference_number)))
      ELSE $7::text
   END,
   (SELECT reference_number FROM reference_number),
   $10,
   $11
WHERE $13::int = 0 OR (SELECT COUNT(*) FROM conversations WHERE tenant_id = $1 AND contact_id = $2 AND created_at >= $12) < $13::int
RETURNING id, uuid;

-- name: get-conversations
-- $1 = tenant
-- $2 = viewing user ID for per-agent unread count
-- $3 = include mentioned message UUID (true for mentioned inbox, false otherwise)
SELECT
    COUNT(*) OVER() as total,
    conversations.id,
    conversations.tenant_id,
    conversations.created_at,
    conversations.updated_at,
    conversations.uuid,
    conversations.reference_number,
    conversations.waiting_since,
    users.created_at as "contact.created_at",
    users.updated_at as "contact.updated_at",
    users.first_name as "contact.first_name",
    users.last_name as "contact.last_name",
    users.email as "contact.email",
    users.avatar_url as "contact.avatar_url",
    inboxes.channel as inbox_channel,
    inboxes.name as inbox_name,
    conversations.sla_policy_id,
    conversations.first_reply_at,
    conversations.last_reply_at,
    conversations.resolved_at,
    conversations.subject,
    conversations.last_message,
    conversations.last_message_at,
    conversations.last_message_sender,
    conversations.last_interaction,
    conversations.last_interaction_at,
    conversations.last_interaction_sender,
    conversations.next_sla_deadline_at,
    conversations.priority_id,
    conversations.assigned_user_id,
    conversations.assigned_team_id,
    (
    SELECT CASE WHEN COUNT(*) > 9 THEN 10 ELSE COUNT(*) END
    FROM (
        SELECT 1 FROM conversation_messages
        WHERE conversation_id = conversations.id
        AND created_at > COALESCE(
            (SELECT last_seen_at FROM conversation_last_seen
             WHERE conversation_id = conversations.id AND user_id = $2),
            '1970-01-01'::TIMESTAMPTZ
        )
        AND (meta IS NULL OR NOT COALESCE((meta->>'continuity_email')::boolean, false))
        LIMIT 10
    ) t
    ) as unread_message_count,
    conversation_statuses.name as status,
    conversation_priorities.name as priority,
    as_latest.first_response_deadline_at,
    as_latest.resolution_deadline_at,
    as_latest.id as applied_sla_id,
    nxt_resp_event.deadline_at AS next_response_deadline_at,
    nxt_resp_event.met_at as next_response_met_at,
    CASE WHEN $3 = true THEN (
        SELECT msg.uuid
        FROM conversation_mentions cm2
        JOIN conversation_messages msg ON msg.id = cm2.message_id
        WHERE cm2.conversation_id = conversations.id
          AND (cm2.mentioned_user_id = $2 OR EXISTS(
              SELECT 1 FROM team_members tm2
              WHERE tm2.team_id = cm2.mentioned_team_id AND tm2.user_id = $2
          ))
        ORDER BY cm2.created_at DESC
        LIMIT 1
    ) ELSE NULL END as mentioned_message_uuid
    FROM conversations
    JOIN users ON contact_id = users.id
    JOIN inboxes ON inbox_id = inboxes.id  
    LEFT JOIN conversation_statuses ON status_id = conversation_statuses.id
    LEFT JOIN conversation_priorities ON priority_id = conversation_priorities.id
    LEFT JOIN LATERAL (
        SELECT id, first_response_deadline_at, resolution_deadline_at
        FROM applied_slas 
        WHERE conversation_id = conversations.id 
        ORDER BY created_at DESC LIMIT 1
    ) as_latest ON true
    LEFT JOIN LATERAL (
        SELECT se.deadline_at, se.met_at
        FROM sla_events se
        WHERE se.applied_sla_id = as_latest.id
        AND se.type = 'next_response'
        ORDER BY se.created_at DESC
        LIMIT 1
    ) nxt_resp_event ON true
WHERE conversations.tenant_id = $1 %s

-- name: get-conversation-list-item
SELECT
    conversations.id,
    conversations.tenant_id,
    conversations.created_at,
    conversations.updated_at,
    conversations.uuid,
    conversations.reference_number,
    conversations.waiting_since,
    users.created_at as "contact.created_at",
    users.updated_at as "contact.updated_at",
    users.first_name as "contact.first_name",
    users.last_name as "contact.last_name",
    users.email as "contact.email",
    users.avatar_url as "contact.avatar_url",
    inboxes.channel as inbox_channel,
    inboxes.name as inbox_name,
    conversations.sla_policy_id,
    conversations.first_reply_at,
    conversations.last_reply_at,
    conversations.resolved_at,
    conversations.subject,
    conversations.last_message,
    conversations.last_message_at,
    conversations.last_message_sender,
    conversations.last_interaction,
    conversations.last_interaction_at,
    conversations.last_interaction_sender,
    conversations.next_sla_deadline_at,
    conversations.priority_id,
    conversations.assigned_user_id,
    conversations.assigned_team_id,
    conversation_statuses.name as status,
    conversation_priorities.name as priority,
    as_latest.first_response_deadline_at,
    as_latest.resolution_deadline_at,
    as_latest.id as applied_sla_id,
    nxt_resp_event.deadline_at AS next_response_deadline_at,
    nxt_resp_event.met_at as next_response_met_at
FROM conversations
JOIN users ON contact_id = users.id
JOIN inboxes ON inbox_id = inboxes.id
LEFT JOIN conversation_statuses ON status_id = conversation_statuses.id
LEFT JOIN conversation_priorities ON priority_id = conversation_priorities.id
LEFT JOIN LATERAL (
    SELECT id, first_response_deadline_at, resolution_deadline_at
    FROM applied_slas
    WHERE conversation_id = conversations.id
    ORDER BY created_at DESC LIMIT 1
) as_latest ON true
LEFT JOIN LATERAL (
    SELECT se.deadline_at, se.met_at
    FROM sla_events se
    WHERE se.applied_sla_id = as_latest.id
    AND se.type = 'next_response'
    ORDER BY se.created_at DESC
    LIMIT 1
) nxt_resp_event ON true
WHERE conversations.tenant_id = $1 AND conversations.uuid = $2::uuid;

-- name: get-conversation
SELECT
   c.id,
   c.tenant_id,
   c.created_at,
   c.updated_at,
   c.closed_at,
   c.resolved_at,
   c.inbox_id,
   inb.name as inbox_name,
   COALESCE(inb.from, '') as inbox_mail,
   COALESCE(inb.config->>'reply_to', '') as inbox_reply_to,
   COALESCE(inb.channel::TEXT, '') as inbox_channel,
   c.status_id,
   c.priority_id,
   p.name as priority,
   s.name as status,
   s.category as status_category,
   c.uuid,
   c.reference_number,
   c.first_reply_at,
   c.last_reply_at,
   c.waiting_since,
   c.snoozed_until,
   c.assigned_user_id,
   c.assigned_team_id,
   c.subject,
   c.contact_id,
   c.sla_policy_id,
   c.meta,
   sla.name as sla_policy_name,
   c.last_message_at,
   c.last_message_sender,
   c.last_message,
   c.last_interaction,
   c.last_interaction_at,
   c.last_interaction_sender,
   c.custom_attributes,
   (SELECT COALESCE(
       (SELECT json_agg(t.name)
       FROM tags t
       INNER JOIN conversation_tags ct ON ct.tag_id = t.id
       WHERE ct.conversation_id = c.id),
       '[]'::json
   )) AS tags,
   ct.id as "contact.id",
   ct.created_at as "contact.created_at",
   ct.updated_at as "contact.updated_at",
   ct.first_name as "contact.first_name",
   ct.last_name as "contact.last_name", 
   ct.email as "contact.email",
   ct.type as "contact.type",
   ct.availability_status as "contact.availability_status",
   ct.avatar_url as "contact.avatar_url",
   ct.phone_number as "contact.phone_number",
   ct.phone_number_country_code as "contact.phone_number_country_code",
   ct.country as "contact.country",
   ct.custom_attributes as "contact.custom_attributes",
   ct.enabled as "contact.enabled",
   ct.last_active_at as "contact.last_active_at",
   ct.last_login_at as "contact.last_login_at",
   ct.external_user_id as "contact.external_user_id",
   as_latest.first_response_deadline_at,
   as_latest.resolution_deadline_at,
   as_latest.id as applied_sla_id,
   nxt_resp_event.deadline_at AS next_response_deadline_at,
   nxt_resp_event.met_at as next_response_met_at,
   c.last_continuity_email_sent_at,
   csat.rating as csat_rating,
   csat.feedback as csat_feedback,
   csat.response_timestamp as csat_responded_at
FROM conversations c
JOIN users ct ON c.contact_id = ct.id
JOIN inboxes inb ON c.inbox_id = inb.id
LEFT JOIN LATERAL (
    SELECT rating, feedback, response_timestamp
    FROM csat_responses
    WHERE conversation_id = c.id
    ORDER BY response_timestamp DESC NULLS LAST, created_at DESC
    LIMIT 1
) csat ON true
LEFT JOIN sla_policies sla ON c.sla_policy_id = sla.id
LEFT JOIN teams at ON at.id = c.assigned_team_id
LEFT JOIN conversation_statuses s ON c.status_id = s.id
LEFT JOIN conversation_priorities p ON c.priority_id = p.id
LEFT JOIN LATERAL (
    SELECT id, first_response_deadline_at, resolution_deadline_at
    FROM applied_slas
    WHERE conversation_id = c.id 
    ORDER BY created_at DESC LIMIT 1
) as_latest ON true
LEFT JOIN LATERAL (
  SELECT se.deadline_at, se.met_at
  FROM sla_events se
  WHERE se.applied_sla_id = as_latest.id
  AND se.type = 'next_response'
  ORDER BY se.created_at DESC
  LIMIT 1
) nxt_resp_event ON true
WHERE c.tenant_id = $1 AND (
  ($2 > 0 AND c.id = $2)
  OR
  (NULLIF($3, '')::uuid IS NOT NULL AND c.uuid = NULLIF($3, '')::uuid)
  OR
  ($4::TEXT != '' AND c.reference_number = $4::TEXT)
)


-- name: get-conversations-created-after
-- tenant: all, time triggers run for every tenant; each row carries its tenant_id.
SELECT
    c.id,
    c.tenant_id,
    c.uuid
FROM conversations c
WHERE c.created_at > $1;

-- name: get-contact-previous-conversations
SELECT
    c.id,
    c.created_at,
    c.updated_at,
    c.uuid,
    c.subject,
    u.first_name AS "contact.first_name",
    u.last_name AS "contact.last_name",
    u.avatar_url AS "contact.avatar_url",
    c.last_message as last_message,
    c.last_message_at as last_message_at
FROM users u
JOIN conversations c ON c.contact_id = u.id
WHERE c.tenant_id = $1 AND c.contact_id = $2
ORDER BY c.created_at DESC
LIMIT $3;

-- name: get-contact-conversations-for-ai
SELECT
    c.id,
    c.reference_number,
    c.subject,
    cs.name AS status,
    c.created_at,
    c.last_message_at,
    c.assigned_user_id,
    c.assigned_team_id
FROM conversations c
LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
WHERE c.tenant_id = $1 AND c.contact_id = $2
  AND c.id != $3
ORDER BY c.created_at DESC
LIMIT 50;

-- name: get-conversations-by-contact-email-for-ai
SELECT
    c.id,
    c.reference_number,
    c.subject,
    cs.name AS status,
    c.created_at,
    c.last_message_at,
    c.assigned_user_id,
    c.assigned_team_id,
    TRIM(CONCAT(u.first_name, ' ', COALESCE(u.last_name, ''))) AS contact_name
FROM conversations c
JOIN users u ON c.contact_id = u.id
LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
WHERE c.tenant_id = $1 AND LOWER(u.email) = LOWER($2)
  AND u.type = 'contact'
  AND u.deleted_at IS NULL
ORDER BY c.created_at DESC
LIMIT 50;

-- name: get-chat-conversation
SELECT
    c.created_at,
    c.uuid,
    cs.name as status,
    COALESCE(c.last_interaction, '') as "last_message.content",
    c.last_interaction_at as "last_message.created_at",
    COALESCE(lis.id, 0) AS "last_message.author.id",
    COALESCE(lis.first_name, '') AS "last_message.author.first_name",
    COALESCE(lis.last_name, '') AS "last_message.author.last_name",
    COALESCE(lis.avatar_url, '') AS "last_message.author.avatar_url",
    COALESCE(c.last_interaction_sender::TEXT, '') AS "last_message.author.type",
    (SELECT CASE WHEN COUNT(*) > 9 THEN 10 ELSE COUNT(*) END
     FROM (
         SELECT 1 FROM conversation_messages unread
         WHERE unread.conversation_id = c.id
           AND unread.created_at > c.contact_last_seen_at
           AND unread.type = 'outgoing'
           AND unread.private = false
         LIMIT 10
     ) t) AS unread_message_count,
    COALESCE(au.availability_status::TEXT, '') as "assignee.availability_status",
    au.avatar_url as "assignee.avatar_url",
    COALESCE(au.first_name, '') as "assignee.first_name",
    COALESCE(au.id, 0) as "assignee.id",
    COALESCE(au.last_name, '') as "assignee.last_name",
    COALESCE(au.type::TEXT, '') as "assignee.type",
    COALESCE(aa.expectation, '') as "assignee.expectation"
FROM conversations c
INNER JOIN inboxes inb on c.inbox_id = inb.id
LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
LEFT JOIN users au ON c.assigned_user_id = au.id
LEFT JOIN ai_assistants aa ON aa.user_id = au.id
LEFT JOIN users lis ON c.last_interaction_sender_id = lis.id
WHERE c.tenant_id = $1 AND c.uuid = $2
  AND inb.deleted_at IS NULL;

-- name: get-contact-chat-conversations
SELECT
    c.created_at,
    c.uuid,
    cs.name as status,
    COALESCE(c.last_interaction, '') as "last_message.content",
    c.last_interaction_at as "last_message.created_at",
    COALESCE(lis.id, 0) AS "last_message.author.id",
    COALESCE(lis.first_name, '') AS "last_message.author.first_name",
    COALESCE(lis.last_name, '') AS "last_message.author.last_name",
    COALESCE(lis.avatar_url, '') AS "last_message.author.avatar_url",
    COALESCE(c.last_interaction_sender::TEXT, '') AS "last_message.author.type",
    (SELECT CASE WHEN COUNT(*) > 9 THEN 10 ELSE COUNT(*) END
     FROM (
         SELECT 1 FROM conversation_messages unread
         WHERE unread.conversation_id = c.id
           AND unread.created_at > c.contact_last_seen_at
           AND unread.type = 'outgoing'
           AND unread.private = false
         LIMIT 10
     ) t) AS unread_message_count,
    COALESCE(au.availability_status::TEXT, '') as "assignee.availability_status",
    au.avatar_url as "assignee.avatar_url",
    COALESCE(au.first_name, '') as "assignee.first_name",
    COALESCE(au.id, 0) as "assignee.id",
    COALESCE(au.last_name, '') as "assignee.last_name",
    COALESCE(au.type::TEXT, '') as "assignee.type"
FROM conversations c
INNER JOIN inboxes inb ON c.inbox_id = inb.id
INNER JOIN users con ON c.contact_id = con.id
LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
LEFT JOIN users au ON c.assigned_user_id = au.id
LEFT JOIN users lis ON c.last_interaction_sender_id = lis.id
WHERE c.tenant_id = $1 AND c.contact_id = $2 AND c.inbox_id = $3
  AND inb.deleted_at IS NULL
  AND con.deleted_at IS NULL
ORDER BY c.created_at DESC
LIMIT 200;

-- name: get-conversation-uuid
SELECT uuid from conversations where tenant_id = $1 AND id = $2;

-- name: update-conversation-assigned-user
UPDATE conversations
SET assigned_user_id = $3,
updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: claim-unassigned-conversation
UPDATE conversations
SET assigned_user_id = $3,
updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2 AND assigned_user_id IS NULL AND assigned_team_id = $4;

-- name: update-conversation-contact-last-seen
UPDATE conversations
SET contact_last_seen_at = NOW(),
updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: update-conversation-assigned-team
UPDATE conversations
SET assigned_team_id = $3,
updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;


-- name: update-conversation-status
WITH new_status AS (
    SELECT id, category FROM conversation_statuses WHERE tenant_id = $1 AND name = $3
)
UPDATE conversations
SET status_id     = (SELECT id FROM new_status),
    resolved_at   = COALESCE(resolved_at, CASE WHEN (SELECT category FROM new_status) = 'resolved' THEN NOW() END),
    closed_at     = COALESCE(closed_at,   CASE WHEN $3 = 'Closed'                                  THEN NOW() END),
    snoozed_until = CASE WHEN $3 = 'Snoozed' THEN $4::timestamptz ELSE NULL END,
    updated_at    = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: get-user-active-conversations-count
SELECT COUNT(*) FROM conversations WHERE tenant_id = $1 AND status_id IN (SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND category = 'open') AND assigned_user_id = $2;

-- name: update-conversation-priority
UPDATE conversations 
SET priority_id = (SELECT id FROM conversation_priorities WHERE tenant_id = $1 AND name = $3),
    updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: upsert-user-last-seen
INSERT INTO conversation_last_seen (tenant_id, user_id, conversation_id, last_seen_at)
VALUES ($1, $2, (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $3), NOW())
ON CONFLICT (conversation_id, user_id)
DO UPDATE SET last_seen_at = NOW(), updated_at = NOW();

-- name: update-conversation-last-message
-- $1=tenant, $2=id, $3=uuid, $4=content, $5=sender_type, $6=timestamp, $7=message_type, $8=private, $9=sender_id
UPDATE conversations SET
    last_message = $4,
    last_message_sender = $5,
    last_message_sender_id = $9,
    last_message_at = $6,
    last_interaction = CASE WHEN $7 != 'activity' AND $8 = false THEN $4 ELSE last_interaction END,
    last_interaction_sender = CASE WHEN $7 != 'activity' AND $8 = false THEN $5 ELSE last_interaction_sender END,
    last_interaction_sender_id = CASE WHEN $7 != 'activity' AND $8 = false THEN $9 ELSE last_interaction_sender_id END,
    last_interaction_at = CASE WHEN $7 != 'activity' AND $8 = false THEN $6 ELSE last_interaction_at END,
    updated_at = NOW()
WHERE tenant_id = $1 AND CASE
    WHEN $2 > 0 THEN id = $2
    ELSE uuid = $3
END

-- name: get-conversation-participants
SELECT users.id as id, first_name, last_name, avatar_url 
FROM conversation_participants
INNER JOIN users ON users.id = conversation_participants.user_id
WHERE conversation_participants.tenant_id = $1 AND conversation_id =
(
    SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $2
);

-- name: insert-conversation-participant
INSERT INTO conversation_participants
(tenant_id, user_id, conversation_id)
VALUES($1, $2, (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $3));

-- name: get-unassigned-conversations
SELECT
    c.created_at,
    c.updated_at,
    c.uuid,
    c.assigned_team_id,
    inb.channel as inbox_channel,
    inb.name as inbox_name
FROM conversations c
    JOIN inboxes inb ON c.inbox_id = inb.id 
WHERE c.tenant_id = $1 AND assigned_user_id IS NULL AND assigned_team_id IS NOT NULL
ORDER BY c.created_at ASC;

-- name: add-conversation-tags
-- Insert new tags
INSERT INTO conversation_tags (tenant_id, conversation_id, tag_id)
  SELECT $1, c.id, t.id
  FROM conversations c, tags t
  WHERE c.tenant_id = $1 AND t.tenant_id = $1 AND t.name = ANY($3::text[]) AND c.uuid = $2
  ON CONFLICT (conversation_id, tag_id) DO UPDATE SET tag_id = EXCLUDED.tag_id;

-- name: set-conversation-tags
WITH conversation_id AS (
    SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $2
),
-- Insert new tags
inserted AS (
    INSERT INTO conversation_tags (tenant_id, conversation_id, tag_id)
    SELECT $1, conversation_id.id, t.id
    FROM conversation_id, tags t
    WHERE t.tenant_id = $1 AND t.name = ANY($3::text[])
    ON CONFLICT (conversation_id, tag_id) DO UPDATE SET tag_id = EXCLUDED.tag_id
)
-- Delete tags that are not in the new list
DELETE FROM conversation_tags
WHERE tenant_id = $1 AND conversation_id = (SELECT id FROM conversation_id)
AND tag_id NOT IN (
    SELECT id FROM tags WHERE tenant_id = $1 AND name = ANY($3::text[])
);

-- name: remove-conversation-tags
-- Delete tags that are not in the new list
DELETE FROM conversation_tags
WHERE tenant_id = $1 AND conversation_id = (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $2)
AND tag_id IN (
    SELECT id FROM tags WHERE tenant_id = $1 AND name = ANY($3::text[])
);

-- name: get-conversation-tags
SELECT t.name
FROM conversation_tags ct
JOIN tags t ON ct.tag_id = t.id
WHERE ct.tenant_id = $1 AND ct.conversation_id = (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $2);

-- name: get-conversation-uuid-from-message-uuid
SELECT c.uuid AS conversation_uuid
FROM conversation_messages m
JOIN conversations c ON m.conversation_id = c.id
WHERE m.tenant_id = $1 AND m.uuid = $2;

-- name: unassign-open-conversations
UPDATE conversations
SET assigned_user_id = NULL,
    updated_at = NOW()
WHERE tenant_id = $1 AND assigned_user_id = $2 AND status_id IN (SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND category != 'resolved');

-- name: update-conversation-custom-attributes
UPDATE conversations
SET custom_attributes = $3,
    updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: update-conversation-waiting-since
UPDATE conversations
SET waiting_since = $3,
    updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: update-conversation-reply-timestamps
WITH old AS (
    SELECT first_reply_at IS NULL AS is_first FROM conversations WHERE tenant_id = $1 AND id = $2
)
UPDATE conversations SET
    first_reply_at = COALESCE(conversations.first_reply_at, $3),
    last_reply_at = $3,
    waiting_since = NULL,
    updated_at = NOW()
FROM old WHERE conversations.tenant_id = $1 AND conversations.id = $2
RETURNING old.is_first AS is_first_reply;

-- name: remove-conversation-assignee
UPDATE conversations
SET
    assigned_user_id = CASE WHEN $3 = 'user' THEN NULL ELSE assigned_user_id END,
    assigned_team_id = CASE WHEN $3 = 'team' THEN NULL ELSE assigned_team_id END,
    updated_at = NOW()
WHERE tenant_id = $1 AND uuid = $2;

-- name: re-open-conversation
-- Open conversation if it is not already open and unset the assigned user if they are away and reassigning.
UPDATE conversations
SET 
  status_id = (SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND name = 'Open'),
  snoozed_until = NULL,
  updated_at = NOW(),
  assigned_user_id = CASE
    WHEN EXISTS (
      SELECT 1 FROM users 
      WHERE users.id = conversations.assigned_user_id 
        AND users.availability_status = 'away_and_reassigning'
    ) THEN NULL
    ELSE assigned_user_id
  END
WHERE
  tenant_id = $1
  AND uuid = $2
  AND status_id IN (
    SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND name NOT IN ('Open')
  )

-- name: get-conversation-by-message-id
SELECT
    c.id,
    c.uuid,
    c.assigned_team_id,
    c.assigned_user_id
FROM conversation_messages m
JOIN conversations c ON m.conversation_id = c.id
WHERE m.tenant_id = $1 AND m.id = $2;

-- name: delete-conversation
DELETE FROM conversations WHERE tenant_id = $1 AND uuid = $2;

-- MESSAGE queries.
-- name: delete-message
DELETE FROM conversation_messages WHERE tenant_id = $1 AND CASE
    WHEN $2 > 0 THEN id = $2
    ELSE uuid = $3
END;

-- name: delete-private-message
-- $1 = tenant, $2 = message uuid, $3 = conversation uuid, $4 = deleted placeholder text, $5 = sender id, 0 to skip the sender check.
WITH deleted AS (
    UPDATE conversation_messages
    SET content = $4, text_content = $4, updated_at = NOW(),
        meta = COALESCE(meta, '{}'::jsonb) || jsonb_build_object('deleted_at', NOW())
    WHERE tenant_id = $1
      AND uuid = $2
      AND private = true
      AND meta->>'deleted_at' IS NULL
      AND ($5 = 0 OR sender_id = $5)
      AND conversation_id = (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $3)
    RETURNING id, conversation_id, created_at
),
media_unlink AS (
    UPDATE media SET model_id = 0
    FROM deleted d
    WHERE media.tenant_id = $1 AND media.model_type = 'messages' AND media.model_id = d.id
),
preview AS (
    UPDATE conversations c
    SET last_message = $4, updated_at = NOW()
    FROM deleted d
    WHERE c.id = d.conversation_id
      AND NOT EXISTS (
          SELECT 1 FROM conversation_messages m2
          WHERE m2.conversation_id = d.conversation_id
            AND m2.created_at > d.created_at
      )
    RETURNING c.id
)
SELECT EXISTS (SELECT 1 FROM preview) AS preview_updated
FROM deleted d;

-- name: get-message-source-ids
SELECT 
    source_id
FROM conversation_messages
WHERE tenant_id = $1 AND conversation_id = $2
AND type in ('incoming', 'outgoing') and private = false
and source_id > ''
ORDER BY id DESC
LIMIT $3;

-- name: get-outgoing-pending-messages
-- tenant: all, the outgoing queue sends messages of every tenant; each row carries its tenant_id.
-- Tenants take turns: every tenant's oldest pending message comes before any
-- tenant's second, so one tenant's backlog doesn't hold up the others. $2
-- caps the batch.
SELECT
    m.id,
    m.tenant_id,
    m.created_at,
    m.updated_at,
    m.status,
    m.type,
    m.content,
    m.text_content,
    m.sender_type,
    m.content_type,
    m.conversation_id,
    m.uuid,
    m.private,
    m.sender_type,
    m.sender_id,
    m.meta,
    c.uuid as conversation_uuid,
    m.content_type,
    m.source_id,
    m.meta,
    ARRAY(SELECT jsonb_array_elements_text(m.meta->'cc')) AS cc,
    ARRAY(SELECT jsonb_array_elements_text(m.meta->'bcc')) AS bcc,
    ARRAY(SELECT jsonb_array_elements_text(m.meta->'to')) AS to,
    c.inbox_id,
    c.uuid as conversation_uuid,
    c.subject,
    c.contact_id as message_receiver_id,
    c.subject
FROM conversation_messages m
INNER JOIN conversations c ON c.id = m.conversation_id
WHERE m.status = 'pending' AND m.type = 'outgoing' AND m.private = false
AND NOT(m.id = ANY($1::INT[]))
ORDER BY ROW_NUMBER() OVER (PARTITION BY m.tenant_id ORDER BY m.id), m.id
LIMIT $2;

-- name: get-message
SELECT
    m.id,
    m.created_at,
    m.updated_at,
    m.status,
    m.type,
    m.content,
    m.text_content,
    m.content_type,
    m.conversation_id,
    m.uuid,
    m.private,
    m.sender_type,
    m.sender_id,
    m.meta,
    c.uuid as conversation_uuid,
    u.id AS "author.id",
    u.first_name AS "author.first_name",
    u.last_name AS "author.last_name",
    u.email AS "author.email",
    u.avatar_url AS "author.avatar_url",
    u.availability_status AS "author.availability_status",
    u.type AS "author.type",
    u.last_active_at AS "author.last_active_at",
    COALESCE(
        json_agg(
            json_build_object(
                'name', media.filename,
                'content_type', media.content_type,
                'uuid', media.uuid,
                'size', media.size,
                'content_id', media.content_id,
                'disposition', media.disposition
            ) ORDER BY media.filename
        ) FILTER (WHERE media.id IS NOT NULL),
        '[]'::json
    ) AS attachments
FROM conversation_messages m
INNER JOIN conversations c ON c.id = m.conversation_id
JOIN users u ON m.sender_id = u.id
LEFT JOIN media ON media.tenant_id = m.tenant_id AND media.model_type = 'messages' AND media.model_id = m.id
WHERE m.tenant_id = $1 AND m.uuid = $2
GROUP BY
    m.id, m.created_at, m.updated_at, m.status, m.type, m.content, m.uuid, m.private, m.sender_type, c.uuid,
    u.id, u.first_name, u.last_name, u.email, u.avatar_url, u.availability_status, u.type, u.last_active_at
ORDER BY m.created_at;

-- name: get-messages
SELECT
   COUNT(*) OVER() AS total,
   m.id,
   m.created_at,
   m.updated_at,
   m.status,
   m.type,
   m.content,
   m.text_content,
   m.content_type,
   m.conversation_id,
   m.uuid,
   m.private,
   m.sender_id,
   m.sender_type,
   m.meta,
   $2::uuid AS conversation_uuid,
   u.id AS "author.id",
   u.first_name AS "author.first_name",
   u.last_name AS "author.last_name",
   u.email AS "author.email",
   u.avatar_url AS "author.avatar_url",
   u.availability_status AS "author.availability_status",
   u.type AS "author.type",
   u.last_active_at AS "author.last_active_at",
   COALESCE(
     (SELECT json_agg(
       json_build_object(
         'name', filename,
         'content_type', content_type,
         'uuid', uuid,
         'size', size,
         'content_id', content_id,
         'disposition', disposition
       ) ORDER BY filename
     ) FROM media
     WHERE media.tenant_id = m.tenant_id AND model_type = 'messages' AND model_id = m.id),
   '[]'::json) AS attachments
FROM conversation_messages m
JOIN users u ON m.sender_id = u.id
WHERE m.tenant_id = $1 AND m.conversation_id = (
   SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $2 LIMIT 1
)
AND ($3::boolean IS NULL OR m.private = $3)
AND ($4::text[] IS NULL OR m.type::text = ANY($4))
AND (m.meta IS NULL OR NOT COALESCE((m.meta->>'continuity_email')::boolean, false))
ORDER BY m.created_at DESC %s

-- name: insert-message
WITH conversation_id AS (
   SELECT id
   FROM conversations
   WHERE tenant_id = $1 AND CASE
       WHEN $4 > 0 THEN id = $4
       ELSE uuid = $5
   END
),
inserted_msg AS (
   INSERT INTO conversation_messages (
       tenant_id, "type", status, conversation_id, "content",
       text_content, sender_id, sender_type, private,
       content_type, source_id, meta, created_at, updated_at
   )
   VALUES (
       $1, $2, $3, (SELECT id FROM conversation_id),
       $6, $7, $8, $9, $10, $11, $12, $13,
       COALESCE($14::TIMESTAMPTZ, NOW()), COALESCE($14::TIMESTAMPTZ, NOW())
   )
   RETURNING *
)
SELECT * FROM inserted_msg;

-- name: message-exists-by-source-id
SELECT conversation_id
FROM conversation_messages
WHERE tenant_id = $1 AND source_id = ANY($2::text []);

-- name: update-message-status
update conversation_messages set status = $2, updated_at = NOW() where tenant_id = $1 AND uuid = $3;

-- name: update-message-source-id
UPDATE conversation_messages SET source_id = $2 WHERE tenant_id = $1 AND id = $3;

-- name: get-offline-livechat-conversations
SELECT
    c.id,
    c.tenant_id,
    c.uuid,
    c.contact_id,
    c.inbox_id,
    c.contact_last_seen_at,
    c.last_continuity_email_sent_at,
    i.linked_email_inbox_id,
    u.email as contact_email,
    u.first_name as contact_first_name,
    u.last_name as contact_last_name,
    c.reference_number,
    c.meta->>'continuity_email_subject' as continuity_email_subject
FROM conversations c
JOIN users u ON u.id = c.contact_id
JOIN inboxes i ON i.id = c.inbox_id
WHERE c.tenant_id = $1
  AND i.channel = 'livechat'
  AND i.enabled = TRUE
  AND i.linked_email_inbox_id IS NOT NULL
  AND c.contact_last_seen_at IS NOT NULL
  AND c.contact_last_seen_at < NOW() - MAKE_INTERVAL(mins => $2)
  AND EXISTS (
    SELECT 1 FROM conversation_messages cm
    WHERE cm.conversation_id = c.id
      AND cm.created_at > c.contact_last_seen_at
      AND cm.type = 'outgoing'
      AND cm.private = false
      AND (cm.meta IS NULL OR NOT COALESCE((cm.meta->>'continuity_email')::boolean, false))
      AND (cm.meta IS NULL OR NOT COALESCE((cm.meta->>'continuity_emailed')::boolean, false))
  )
  AND u.email > ''
  AND (c.last_continuity_email_sent_at IS NULL
       OR c.last_continuity_email_sent_at < NOW() - MAKE_INTERVAL(mins => $3))
  AND c.inbox_id = $4;

-- name: get-unread-messages
SELECT
    m.id,
    m.created_at,
    m.updated_at,
    m.status,
    m.type,
    m.content,
    m.text_content,
    m.uuid,
    m.private,
    m.sender_id,
    m.sender_type,
    m.meta,
    u.first_name as "sender.first_name",
    u.last_name as "sender.last_name",
    u.type as "sender.type",
    COALESCE((SELECT string_agg(md.filename, ',') FROM media md WHERE md.tenant_id = m.tenant_id AND md.model_id = m.id AND md.model_type = 'messages'), '') AS attachment_names
FROM conversation_messages m
LEFT JOIN users u ON u.id = m.sender_id
WHERE m.tenant_id = $1
  AND m.conversation_id = $2
  AND m.created_at > $3
  AND m.type = 'outgoing'
  AND m.private = false
  AND (m.meta IS NULL OR NOT COALESCE((m.meta->>'continuity_email')::boolean, false))
  AND (m.meta IS NULL OR NOT COALESCE((m.meta->>'continuity_emailed')::boolean, false))
ORDER BY m.created_at ASC
LIMIT $4;

-- name: mark-messages-continuity-emailed
UPDATE conversation_messages
SET meta = COALESCE(meta, '{}'::jsonb) || '{"continuity_emailed": true}'::jsonb
WHERE tenant_id = $1 AND id = ANY($2::int[]);

-- name: update-continuity-email-tracking
UPDATE conversations
SET last_continuity_email_sent_at = NOW(),
    meta = jsonb_set(COALESCE(meta, '{}'::jsonb), '{continuity_email_subject}', to_jsonb($3::text))
WHERE tenant_id = $1 AND id = $2;

-- name: upsert-conversation-draft
INSERT INTO conversation_drafts (tenant_id, conversation_id, user_id, type, content, meta, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT (conversation_id, user_id, type)
DO UPDATE SET content = EXCLUDED.content, meta = EXCLUDED.meta, updated_at = NOW()
RETURNING *;

-- name: get-all-user-drafts
SELECT cd.id, cd.conversation_id, cd.user_id, cd.type, cd.content, cd.meta, cd.created_at, cd.updated_at, c.uuid as conversation_uuid
FROM conversation_drafts cd
INNER JOIN conversations c ON cd.conversation_id = c.id
WHERE cd.tenant_id = $1 AND cd.user_id = $2
ORDER BY cd.updated_at DESC;

-- name: delete-conversation-draft
DELETE FROM conversation_drafts
WHERE tenant_id = $1 AND conversation_id IN (
  SELECT id FROM conversations
  WHERE tenant_id = $1 AND (($2 > 0 AND id = $2) OR (NULLIF($3, '')::uuid IS NOT NULL AND uuid = NULLIF($3, '')::uuid))
) AND user_id = $4
AND ($5::text = '' OR type = $5::text);

-- name: delete-stale-drafts
-- tenant: all, the draft cleaner applies to every tenant.
DELETE FROM conversation_drafts
WHERE created_at < $1;

-- name: insert-mention
INSERT INTO conversation_mentions (tenant_id, conversation_id, message_id, mentioned_user_id, mentioned_team_id, mentioned_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: mark-conversation-unread
WITH target AS (
    SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $3
),
last_msg AS (
    SELECT created_at - INTERVAL '1 second' AS ts
    FROM conversation_messages
    WHERE conversation_id = (SELECT id FROM target)
      AND (meta IS NULL OR NOT COALESCE((meta->>'continuity_email')::boolean, false))
    ORDER BY created_at DESC LIMIT 1
)
INSERT INTO conversation_last_seen (tenant_id, user_id, conversation_id, last_seen_at)
SELECT $1, $2, (SELECT id FROM target), ts FROM last_msg
ON CONFLICT (conversation_id, user_id)
DO UPDATE SET
    last_seen_at = EXCLUDED.last_seen_at,
    updated_at = NOW();

-- name: get-active-livechat-conversations-by-agent
SELECT c.uuid, c.contact_id, c.inbox_id
FROM conversations c
JOIN inboxes i ON i.id = c.inbox_id
JOIN users u ON u.id = c.contact_id
WHERE c.tenant_id = $1
  AND c.assigned_user_id = $2
  AND i.channel = 'livechat'
  AND i.enabled = TRUE
  AND i.deleted_at IS NULL
  AND u.availability_status = 'online'
ORDER BY c.last_interaction_at DESC
LIMIT 50;

-- name: filter-authorized-list-uuids
-- $1: tenant
-- $2: uuids (uuid[])
-- $3: user_id
-- $4: team_ids (int[])
-- $5: has 'conversations:read'
-- $6: has 'conversations:read_all'
-- $7: has 'conversations:read_assigned'
-- $8: has 'conversations:read_team_all'
-- $9: has 'conversations:read_team_inbox'
-- $10: has 'conversations:read_unassigned'
SELECT uuid::text
FROM conversations
WHERE tenant_id = $1
  AND uuid = ANY($2::uuid[])
  AND $5
  AND (
       $6
    OR ($7 AND assigned_user_id = $3)
    OR ($8 AND assigned_team_id = ANY($4::int[]))
    OR ($9 AND assigned_team_id = ANY($4::int[]) AND assigned_user_id IS NULL)
    OR ($10 AND assigned_user_id IS NULL AND assigned_team_id IS NULL)
  );

-- name: get-conversation-uuids-by-contact
SELECT uuid::text
FROM conversations
WHERE tenant_id = $1 AND contact_id = $2
ORDER BY last_message_at DESC NULLS LAST
LIMIT 200;

-- name: get-unresolved-contact-conversations
-- Every conversation a contact has in an inbox that isn't resolved yet, for
-- starting their chat afresh.
SELECT c.uuid
FROM conversations c
JOIN conversation_statuses cs ON cs.id = c.status_id
WHERE c.tenant_id = $1
  AND c.contact_id = $2
  AND c.inbox_id = $3
  AND cs.category != 'resolved'
ORDER BY c.id;

-- name: get-latest-unresolved-conversation
-- Latest conversation for a contact in an inbox that isn't resolved yet. Used to
-- thread channels without reply headers, e.g. WhatsApp, where a chat is one thread.
SELECT c.id
FROM conversations c
JOIN conversation_statuses cs ON cs.id = c.status_id
WHERE c.tenant_id = $1
  AND c.contact_id = $2
  AND c.inbox_id = $3
  AND cs.category != 'resolved'
ORDER BY c.last_message_at DESC NULLS LAST, c.id DESC
LIMIT 1;

-- name: get-last-incoming-at
-- When a contact last wrote to an inbox, in any of their conversations there.
-- WhatsApp's 24-hour reply window runs from it.
SELECT MAX(m.created_at)
FROM conversation_messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE c.tenant_id = $1
  AND m.tenant_id = $1
  AND c.contact_id = $2
  AND c.inbox_id = $3
  AND m.type = 'incoming';

-- name: insert-message-delivery
INSERT INTO message_deliveries (tenant_id, message_id, inbox_id, recipient, external_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (inbox_id, external_id) DO NOTHING;

-- name: apply-message-delivery
-- Moves a sent message forward: sent, then delivered, then read. Receipts can
-- arrive out of order, so one never moves a message back; failed is final.
UPDATE message_deliveries
SET status = $4, error = NULLIF($5, ''), updated_at = NOW()
WHERE tenant_id = $1 AND inbox_id = $2 AND external_id = $3
  AND (CASE status WHEN 'sent' THEN 1 WHEN 'delivered' THEN 2 WHEN 'read' THEN 3 ELSE 4 END)
    < (CASE $4 WHEN 'sent' THEN 1 WHEN 'delivered' THEN 2 WHEN 'read' THEN 3 ELSE 4 END)
RETURNING message_id;

-- name: apply-message-read-watermark
-- A Messenger read receipt names no message: everything sent to the person up
-- to its watermark has been read.
UPDATE message_deliveries
SET status = 'read', updated_at = NOW()
WHERE tenant_id = $1 AND inbox_id = $2 AND recipient = $3 AND created_at <= $4
  AND status IN ('sent', 'delivered')
RETURNING message_id;

-- name: get-message-deliveries
SELECT status, COALESCE(error, '') AS error FROM message_deliveries
WHERE tenant_id = $1 AND message_id = $2;

-- name: set-message-delivery
-- Keeps a message's combined delivery state on its meta, so messages load it
-- without a join.
UPDATE conversation_messages m
SET meta = COALESCE(m.meta, '{}'::jsonb) || jsonb_build_object('delivery', $3::jsonb), updated_at = NOW()
FROM conversations c
WHERE m.tenant_id = $1 AND m.id = $2 AND c.tenant_id = $1 AND c.id = m.conversation_id
RETURNING m.uuid, m.status, m.meta, c.uuid AS conversation_uuid;

-- name: finish-imported-conversation
-- Dates an imported conversation from its first message, and resolves it when
-- asked, so old history doesn't land in the agents' open queue.
UPDATE conversations c
SET created_at = LEAST(c.created_at, COALESCE((SELECT MIN(m.created_at) FROM conversation_messages m WHERE m.tenant_id = $1 AND m.conversation_id = c.id), c.created_at)),
    status_id = CASE WHEN $3 THEN COALESCE((SELECT s.id FROM conversation_statuses s WHERE s.tenant_id = $1 AND s.category = 'resolved' ORDER BY s.id LIMIT 1), c.status_id) ELSE c.status_id END,
    resolved_at = CASE WHEN $3 THEN COALESCE(c.last_message_at, NOW()) ELSE c.resolved_at END,
    updated_at = NOW()
WHERE c.tenant_id = $1 AND c.id = $2;

-- name: get-message-by-source-id
SELECT m.id, m.uuid, m.meta, c.uuid AS conversation_uuid, c.id AS conversation_id
FROM conversation_messages m
JOIN conversations c ON c.id = m.conversation_id AND c.tenant_id = m.tenant_id
WHERE m.tenant_id = $1 AND m.source_id = $2;

-- name: update-message-content-meta
UPDATE conversation_messages
SET content = $3, text_content = $4, meta = $5, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2;

-- name: set-message-meta-key
-- Sets one key of a message's meta, e.g. a call's AI summary.
UPDATE conversation_messages m
SET meta = COALESCE(m.meta, '{}'::jsonb) || jsonb_build_object($3::TEXT, $4::JSONB), updated_at = NOW()
FROM conversations c
WHERE m.tenant_id = $1 AND m.uuid = $2 AND c.tenant_id = m.tenant_id AND c.id = m.conversation_id
RETURNING m.meta, c.uuid AS conversation_uuid;

-- name: get-latest-contact-inbox-conversation
-- A contact's newest conversation in an inbox that isn't resolved or closed,
-- to log a call to them on.
SELECT c.uuid
FROM conversations c
JOIN conversation_statuses cs ON cs.id = c.status_id
WHERE c.tenant_id = $1 AND c.contact_id = $2 AND c.inbox_id = $3
  AND cs.category != 'resolved'
ORDER BY c.created_at DESC
LIMIT 1;
