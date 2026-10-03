-- name: get-all
SELECT JSON_OBJECT_AGG(key, value) AS settings FROM (SELECT * FROM settings WHERE tenant_id = $1 ORDER BY key) t;

-- name: update
UPDATE settings AS s
SET value = c.value,
    updated_at = now()
FROM (SELECT * FROM jsonb_each($2)) AS c(key, value)
WHERE s.tenant_id = $1 AND s.key = c.key;

-- name: get-by-prefix
SELECT JSON_OBJECT_AGG(key, value) AS settings 
FROM settings 
WHERE tenant_id = $1 AND key LIKE $2 || '%';

-- name: get
SELECT value FROM settings WHERE tenant_id = $1 AND key = $2;
