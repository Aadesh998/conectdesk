-- name: get-business-hours
SELECT id,
    created_at,
    updated_at,
    "name",
    description,
    is_always_open,
    hours,
    holidays
FROM business_hours
WHERE tenant_id = $1 AND id = $2;

-- name: get-all-business-hours
SELECT id,
    created_at,
    updated_at,
    "name",
    description,
    is_always_open,
    hours,
    holidays
FROM business_hours
WHERE tenant_id = $1
ORDER BY updated_at DESC;

-- name: insert-business-hours
INSERT INTO business_hours (
        tenant_id,
        "name",
        description,
        is_always_open,
        hours,
        holidays
    )
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: delete-business-hours
DELETE FROM business_hours
WHERE tenant_id = $1 AND id = $2;

-- name: update-business-hours
UPDATE business_hours
SET "name" = $3,
    description = $4,
    is_always_open = $5,
    hours = $6,
    holidays = $7,
    updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;
