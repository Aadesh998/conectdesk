-- name: get
SELECT
    id,
    created_at,
    updated_at,
    name,
    actions,
    visibility,
    visible_when,
    message_content,
    user_id,
    team_id,
    usage_count
FROM
    macros
WHERE
    tenant_id = $1 AND id = $2;

-- name: get-all
SELECT
    id,
    created_at,
    updated_at,
    name,
    actions,
    visibility,
    visible_when,
    message_content,
    user_id,
    team_id,
    usage_count
FROM
    macros
WHERE
    tenant_id = $1
ORDER BY
    updated_at DESC;

-- name: create
INSERT INTO
    macros (tenant_id, name, message_content, user_id, team_id, visibility, visible_when, actions)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: update
UPDATE
    macros
SET
    name = $3,
    message_content = $4,
    user_id = $5,
    team_id = $6,
    visibility = $7,
    visible_when = $8,
    actions = $9,
    updated_at = NOW()
WHERE
    tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete
DELETE FROM
    macros
WHERE
    tenant_id = $1 AND id = $2;

-- name: increment-usage-count
UPDATE
    macros
SET
    usage_count = usage_count + 1
WHERE
    tenant_id = $1 AND id = $2;
