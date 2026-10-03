-- name: get-all-context-links
SELECT
    id,
    created_at,
    updated_at,
    name,
    url_template,
    signing_secret,
    token_expiry_seconds,
    is_active
FROM
    context_links
WHERE
    tenant_id = $1
ORDER BY created_at DESC;

-- name: get-context-link
SELECT
    id,
    created_at,
    updated_at,
    name,
    url_template,
    signing_secret,
    token_expiry_seconds,
    is_active
FROM
    context_links
WHERE
    tenant_id = $1 AND id = $2;

-- name: get-context-link-signing-secret
SELECT
    signing_secret
FROM
    context_links
WHERE
    tenant_id = $1 AND id = $2;

-- name: get-active-context-links
SELECT
    id,
    created_at,
    updated_at,
    name,
    url_template,
    token_expiry_seconds,
    is_active
FROM
    context_links
WHERE
    tenant_id = $1 AND is_active = true
ORDER BY created_at DESC;

-- name: insert-context-link
INSERT INTO
    context_links (tenant_id, name, url_template, signing_secret, token_expiry_seconds, is_active)
VALUES
    ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: update-context-link
UPDATE
    context_links
SET
    name = $3,
    url_template = $4,
    signing_secret = $5,
    token_expiry_seconds = $6,
    is_active = $7,
    updated_at = NOW()
WHERE
    tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-context-link
DELETE FROM
    context_links
WHERE
    tenant_id = $1 AND id = $2;

-- name: toggle-context-link
UPDATE
    context_links
SET
    is_active = NOT is_active,
    updated_at = NOW()
WHERE
    tenant_id = $1 AND id = $2
RETURNING *;
