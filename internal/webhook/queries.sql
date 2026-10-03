-- name: get-webhooks-compact
SELECT id, name FROM webhooks WHERE tenant_id = $1 ORDER BY name;

-- name: get-all-webhooks
SELECT
    id,
    created_at,
    updated_at,
    name,
    url,
    kind,
    events,
    secret,
    is_active
FROM
    webhooks
WHERE
    tenant_id = $1
ORDER BY created_at DESC;

-- name: get-webhook
SELECT
    id,
    created_at,
    updated_at,
    name,
    url,
    kind,
    events,
    secret,
    is_active
FROM
    webhooks
WHERE
    tenant_id = $1 AND id = $2;

-- name: get-webhook-secret
SELECT
    secret
FROM
    webhooks
WHERE
    tenant_id = $1 AND id = $2;

-- name: get-active-webhooks
SELECT
    id,
    created_at,
    updated_at,
    name,
    url,
    kind,
    events,
    secret,
    is_active
FROM
    webhooks
WHERE
    tenant_id = $1 AND is_active = true
ORDER BY created_at DESC;

-- name: get-webhooks-by-event
SELECT
    id,
    created_at,
    updated_at,
    name,
    url,
    kind,
    events,
    secret,
    is_active
FROM
    webhooks
WHERE
    tenant_id = $1 AND
    is_active = true AND
    $2 = ANY(events);

-- name: insert-webhook
INSERT INTO
    webhooks (tenant_id, name, url, events, secret, is_active, kind)
VALUES
    ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: update-webhook
UPDATE
    webhooks
SET
    name = $3,
    url = $4,
    events = $5,
    secret = $6,
    is_active = $7,
    kind = $8,
    updated_at = NOW()
WHERE
    tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-webhook
DELETE FROM
    webhooks
WHERE
    tenant_id = $1 AND id = $2;

-- name: toggle-webhook
UPDATE
    webhooks
SET
    is_active = NOT is_active,
    updated_at = NOW()
WHERE
    tenant_id = $1 AND id = $2
RETURNING *;
