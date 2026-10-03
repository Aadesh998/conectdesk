-- name: set-stage
-- Moves a contact to stage $3, recording the move; nothing happens when
-- they're already on it. Returns the stage they were on.
WITH old AS (
	SELECT id, lead_stage FROM users
	WHERE tenant_id = $1 AND id = $2 AND type = 'contact' AND deleted_at IS NULL
	FOR UPDATE
), moved AS (
	UPDATE users u SET lead_stage = $3, lead_stage_changed_at = NOW(), updated_at = NOW()
	FROM old
	WHERE u.tenant_id = $1 AND u.id = old.id AND old.lead_stage IS DISTINCT FROM $3
	RETURNING old.lead_stage AS from_stage
)
INSERT INTO lead_stage_events (tenant_id, contact_id, from_stage, to_stage, actor_id)
SELECT $1, $2, from_stage, $3, $4 FROM moved
RETURNING from_stage;

-- name: clear-stage
-- Takes a contact off the pipeline.
UPDATE users SET lead_stage = NULL, lead_stage_changed_at = NULL, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND type = 'contact' AND deleted_at IS NULL;

-- name: ensure-stage
-- Puts a contact who isn't on the pipeline on stage $3, as when they first
-- get in touch. Returns nothing when they already are.
WITH moved AS (
	UPDATE users SET lead_stage = $3, lead_stage_changed_at = NOW(), updated_at = NOW()
	WHERE tenant_id = $1 AND id = $2 AND type = 'contact' AND deleted_at IS NULL AND lead_stage IS NULL
	RETURNING id
)
INSERT INTO lead_stage_events (tenant_id, contact_id, from_stage, to_stage)
SELECT $1, id, NULL, $3 FROM moved
RETURNING to_stage;

-- name: get-contact-lead
SELECT lead_stage, lead_stage_changed_at FROM users
WHERE tenant_id = $1 AND id = $2 AND type IN ('contact', 'visitor') AND deleted_at IS NULL;

-- name: get-history
SELECT e.id, e.created_at, e.from_stage, e.to_stage, e.actor_id,
	NULLIF(TRIM(CONCAT(a.first_name, ' ', a.last_name)), '') AS actor_name
FROM lead_stage_events e
LEFT JOIN users a ON a.tenant_id = e.tenant_id AND a.id = e.actor_id
WHERE e.tenant_id = $1 AND e.contact_id = $2
ORDER BY e.created_at DESC, e.id DESC
LIMIT 50;

-- name: get-board
-- The newest-moved $3 contacts of each stage in $2, with each stage's count,
-- their latest conversation and whether a call-back of theirs is waiting. $4
-- filters by name, email or number.
WITH leads AS (
	SELECT u.id, u.first_name, COALESCE(u.last_name, '') AS last_name, u.email, u.phone_number, u.avatar_url,
		u.custom_attributes, u.lead_stage, u.lead_stage_changed_at,
		ROW_NUMBER() OVER (PARTITION BY u.lead_stage ORDER BY u.lead_stage_changed_at DESC NULLS LAST, u.id DESC) AS rn,
		COUNT(*) OVER (PARTITION BY u.lead_stage) AS stage_count
	FROM users u
	WHERE u.tenant_id = $1 AND u.type = 'contact' AND u.deleted_at IS NULL AND u.lead_stage = ANY($2::TEXT[])
		AND ($4 = '' OR CONCAT_WS(' ', u.first_name, u.last_name, u.email, u.phone_number) ILIKE '%' || $4 || '%')
)
SELECT l.id, l.first_name, l.last_name, l.email, l.phone_number, l.avatar_url, l.custom_attributes,
	l.lead_stage, l.lead_stage_changed_at, l.stage_count,
	lc.uuid::TEXT AS last_conversation_uuid, lc.last_message_at,
	EXISTS (SELECT 1 FROM call_backs cb WHERE cb.tenant_id = $1 AND cb.contact_id = l.id AND cb.status = 'pending') AS pending_call_back
FROM leads l
LEFT JOIN LATERAL (
	SELECT c.uuid, c.last_message_at FROM conversations c
	WHERE c.tenant_id = $1 AND c.contact_id = l.id
	ORDER BY c.last_message_at DESC NULLS LAST LIMIT 1
) lc ON TRUE
WHERE l.rn <= $3
ORDER BY l.lead_stage, l.rn;

-- name: count-by-stage
SELECT lead_stage, COUNT(*) FROM users
WHERE tenant_id = $1 AND type = 'contact' AND deleted_at IS NULL AND lead_stage IS NOT NULL
GROUP BY lead_stage;

-- name: count-moves
-- How many contacts moved onto one of the stages $2 between $3 and $4, each
-- counted once.
SELECT COUNT(DISTINCT contact_id) FROM lead_stage_events
WHERE tenant_id = $1 AND to_stage = ANY($2::TEXT[]) AND created_at >= $3 AND created_at < $4;
