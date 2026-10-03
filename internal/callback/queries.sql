-- name: record-missed
-- A missed call opens a call-back for the contact, or adds to the open one.
-- The same call reported twice (the unanswered and voicemail steps of one
-- flow, or a retried webhook) counts once. inserted is false for an open one.
INSERT INTO call_backs (tenant_id, contact_id, conversation_id, inbox_id, phone, last_call_sid, voicemail, due_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW() + make_interval(secs => $8::double precision))
ON CONFLICT (tenant_id, contact_id) WHERE status = 'pending'
DO UPDATE SET
	missed_calls = call_backs.missed_calls + CASE WHEN call_backs.last_call_sid = EXCLUDED.last_call_sid THEN 0 ELSE 1 END,
	last_missed_at = CASE WHEN call_backs.last_call_sid = EXCLUDED.last_call_sid THEN call_backs.last_missed_at ELSE NOW() END,
	voicemail = call_backs.voicemail OR EXCLUDED.voicemail,
	last_call_sid = EXCLUDED.last_call_sid,
	conversation_id = EXCLUDED.conversation_id,
	updated_at = NOW()
RETURNING id, (xmax = 0) AS inserted;

-- name: close-for-contact
UPDATE call_backs
SET status = 'done', done_at = NOW(), done_reason = $3, done_by_id = $4, updated_at = NOW()
WHERE tenant_id = $1 AND contact_id = $2 AND status = 'pending'
RETURNING id;

-- name: mark-done
UPDATE call_backs
SET status = 'done', done_at = NOW(), done_reason = 'manual', done_by_id = $3, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND status = 'pending'
RETURNING id;

-- name: add-attempt
UPDATE call_backs SET attempts = attempts + 1, updated_at = NOW()
WHERE tenant_id = $1 AND contact_id = $2 AND status = 'pending'
RETURNING id;

-- name: set-whatsapp
UPDATE call_backs
SET whatsapp_message_id = $3,
	whatsapp_sent_at = CASE WHEN $3::BIGINT IS NOT NULL THEN NOW() ELSE whatsapp_sent_at END,
	whatsapp_error = $4,
	updated_at = NOW()
WHERE tenant_id = $1 AND id = $2;

-- name: whatsapp-sent-since
-- Whether the contact was already sent a missed-call message since $3.
SELECT EXISTS (
	SELECT 1 FROM call_backs
	WHERE tenant_id = $1 AND contact_id = $2 AND whatsapp_sent_at > $3
);

-- name: get
SELECT cb.*,
	c.first_name AS contact_first_name, COALESCE(c.last_name, '') AS contact_last_name,
	conv.uuid AS conversation_uuid, conv.reference_number,
	conv.assigned_user_id, conv.assigned_team_id,
	NULLIF(TRIM(CONCAT(au.first_name, ' ', au.last_name)), '') AS assignee_name,
	t.name AS team_name, i.name AS inbox_name,
	COALESCE(md.status, wm.status::TEXT) AS whatsapp_status,
	wc.uuid::TEXT AS whatsapp_conversation_uuid,
	0 AS total
FROM call_backs cb
JOIN users c ON c.tenant_id = cb.tenant_id AND c.id = cb.contact_id
JOIN conversations conv ON conv.tenant_id = cb.tenant_id AND conv.id = cb.conversation_id
JOIN inboxes i ON i.tenant_id = cb.tenant_id AND i.id = cb.inbox_id
LEFT JOIN users au ON au.tenant_id = cb.tenant_id AND au.id = conv.assigned_user_id
LEFT JOIN teams t ON t.tenant_id = cb.tenant_id AND t.id = conv.assigned_team_id
LEFT JOIN conversation_messages wm ON wm.tenant_id = cb.tenant_id AND wm.id = cb.whatsapp_message_id
LEFT JOIN conversations wc ON wc.tenant_id = cb.tenant_id AND wc.id = wm.conversation_id
LEFT JOIN LATERAL (
	SELECT d.status FROM message_deliveries d WHERE d.tenant_id = cb.tenant_id AND d.message_id = wm.id
	ORDER BY d.updated_at DESC LIMIT 1
) md ON TRUE
WHERE cb.tenant_id = $1 AND cb.id = $2;

-- name: list
-- The call-backs an agent may see, the same as the phone conversations they
-- may read: $3 all; $5 their own ($4); their teams' ($6) with $7 all of them
-- or $8 only unassigned ones; $9 neither assigned to an agent nor a team.
-- Waiting ones come most overdue first, done ones most recent first.
SELECT cb.*,
	c.first_name AS contact_first_name, COALESCE(c.last_name, '') AS contact_last_name,
	conv.uuid AS conversation_uuid, conv.reference_number,
	conv.assigned_user_id, conv.assigned_team_id,
	NULLIF(TRIM(CONCAT(au.first_name, ' ', au.last_name)), '') AS assignee_name,
	t.name AS team_name, i.name AS inbox_name,
	COALESCE(md.status, wm.status::TEXT) AS whatsapp_status,
	wc.uuid::TEXT AS whatsapp_conversation_uuid,
	COUNT(*) OVER () AS total
FROM call_backs cb
JOIN users c ON c.tenant_id = cb.tenant_id AND c.id = cb.contact_id
JOIN conversations conv ON conv.tenant_id = cb.tenant_id AND conv.id = cb.conversation_id
JOIN inboxes i ON i.tenant_id = cb.tenant_id AND i.id = cb.inbox_id
LEFT JOIN users au ON au.tenant_id = cb.tenant_id AND au.id = conv.assigned_user_id
LEFT JOIN teams t ON t.tenant_id = cb.tenant_id AND t.id = conv.assigned_team_id
LEFT JOIN conversation_messages wm ON wm.tenant_id = cb.tenant_id AND wm.id = cb.whatsapp_message_id
LEFT JOIN conversations wc ON wc.tenant_id = cb.tenant_id AND wc.id = wm.conversation_id
LEFT JOIN LATERAL (
	SELECT d.status FROM message_deliveries d WHERE d.tenant_id = cb.tenant_id AND d.message_id = wm.id
	ORDER BY d.updated_at DESC LIMIT 1
) md ON TRUE
WHERE cb.tenant_id = $1 AND cb.status = $2
	AND ($3::BOOLEAN
		OR ($5::BOOLEAN AND conv.assigned_user_id = $4)
		OR (conv.assigned_team_id = ANY($6::INT[]) AND ($7::BOOLEAN OR ($8::BOOLEAN AND conv.assigned_user_id IS NULL)))
		OR ($9::BOOLEAN AND conv.assigned_user_id IS NULL AND conv.assigned_team_id IS NULL))
	AND (NOT $10::BOOLEAN OR conv.assigned_user_id = $4)
ORDER BY (CASE WHEN cb.status = 'pending' THEN cb.due_at END) ASC NULLS LAST, cb.done_at DESC NULLS LAST, cb.id DESC
LIMIT $11 OFFSET $12;

-- name: counts
-- Waiting and overdue call-backs an agent may see, as in list.
SELECT COUNT(*) AS pending, COUNT(*) FILTER (WHERE cb.due_at < NOW()) AS overdue
FROM call_backs cb
JOIN conversations conv ON conv.tenant_id = cb.tenant_id AND conv.id = cb.conversation_id
WHERE cb.tenant_id = $1 AND cb.status = 'pending'
	AND ($2::BOOLEAN
		OR ($4::BOOLEAN AND conv.assigned_user_id = $3)
		OR (conv.assigned_team_id = ANY($5::INT[]) AND ($6::BOOLEAN OR ($7::BOOLEAN AND conv.assigned_user_id IS NULL)))
		OR ($8::BOOLEAN AND conv.assigned_user_id IS NULL AND conv.assigned_team_id IS NULL));

-- name: pending-for-contact
SELECT id FROM call_backs WHERE tenant_id = $1 AND contact_id = $2 AND status = 'pending';

-- name: claim-overdue
-- Marks call-backs that just went overdue as escalated and returns them, each
-- once however many instances run: a row another instance is claiming is
-- skipped.
WITH due AS (
	SELECT id FROM call_backs
	WHERE status = 'pending' AND escalated_at IS NULL AND due_at < NOW()
	ORDER BY due_at
	LIMIT 200
	FOR UPDATE SKIP LOCKED
)
UPDATE call_backs cb SET escalated_at = NOW(), updated_at = NOW()
FROM due
WHERE cb.id = due.id
RETURNING cb.tenant_id, cb.id;

-- name: get-manager-ids
-- The agents who see reports, told when a call-back goes overdue.
SELECT DISTINCT u.id FROM users u
JOIN user_roles ur ON ur.tenant_id = u.tenant_id AND ur.user_id = u.id
JOIN roles r ON r.tenant_id = ur.tenant_id AND r.id = ur.role_id
WHERE u.tenant_id = $1 AND u.type = 'agent' AND u.deleted_at IS NULL AND u.enabled
	AND 'reports:manage' = ANY(r.permissions);
