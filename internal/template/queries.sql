-- name: insert
INSERT INTO templates (tenant_id, "name", body, is_default, subject, type)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: update
WITH u AS (
    UPDATE templates
    SET 
        name = CASE WHEN $7::template_type = 'email_outgoing' THEN $3 ELSE name END,
        body = $4,
        is_default = $5,
        subject = $6,
        type = $7::template_type,
        updated_at = NOW()
    WHERE tenant_id = $1 AND id = $2
    RETURNING *
)
SELECT * FROM u LIMIT 1;

-- name: get-default
SELECT id, created_at, updated_at, type, body, is_default, name, subject, is_builtin FROM templates WHERE tenant_id = $1 AND is_default is TRUE;

-- name: get-all
SELECT id, created_at, updated_at, type, body, is_default, name, subject, is_builtin FROM templates WHERE tenant_id = $1 AND type = $2 ORDER BY updated_at DESC;

-- name: get-template
SELECT id, created_at, updated_at, type, body, is_default, name, subject, is_builtin FROM templates WHERE tenant_id = $1 AND id = $2;

-- name: delete
DELETE FROM templates WHERE tenant_id = $1 AND id = $2;

-- name: get-by-name
SELECT id, created_at, updated_at, type, body, is_default, name, subject, is_builtin FROM templates WHERE tenant_id = $1 AND name = $2;

-- name: is-builtin
SELECT EXISTS(SELECT 1 FROM templates WHERE tenant_id = $1 AND id = $2 AND is_builtin is TRUE);
