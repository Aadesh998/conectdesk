-- name: insert
INSERT INTO csat_responses (tenant_id, conversation_id)
SELECT $1, $2
WHERE NOT EXISTS (SELECT 1 FROM csat_responses WHERE tenant_id = $1 AND conversation_id = $2)
RETURNING uuid;

-- name: get
SELECT id,
    uuid,
    created_at,
    updated_at,
    conversation_id,
    rating,
    feedback,
    meta,
    response_timestamp
FROM csat_responses
WHERE tenant_id = $1 AND uuid = $2;

-- name: update
UPDATE csat_responses
SET rating = $3,
    feedback = $4,
    meta = COALESCE($5::jsonb, '{}'),
    response_timestamp = NOW()
WHERE tenant_id = $1 AND uuid = $2;
