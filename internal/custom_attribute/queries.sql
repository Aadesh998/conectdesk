-- name: get-all-custom-attributes
SELECT
    id,
    created_at,
    updated_at,
    name,
    description,
    applies_to,
    key,
    values,
    data_type,
    regex,
    regex_hint
FROM
    custom_attribute_definitions
WHERE
    tenant_id = $1
    AND CASE WHEN $2 = '' THEN TRUE
         ELSE applies_to = $2
    END
ORDER BY
    updated_at DESC;

-- name: get-custom-attribute
SELECT
    id,
    created_at,
    updated_at,
    name,
    description,
    applies_to,
    key,
    values,
    data_type,
    regex,
    regex_hint
FROM
    custom_attribute_definitions
WHERE
    tenant_id = $1 AND id = $2;

-- name: insert-custom-attribute
INSERT INTO
    custom_attribute_definitions (tenant_id, applies_to, name, description, key, values, data_type, regex, regex_hint)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *

-- name: delete-custom-attribute
DELETE FROM
    custom_attribute_definitions
WHERE
    tenant_id = $1 AND id = $2;

-- name: update-custom-attribute
UPDATE
    custom_attribute_definitions
SET
    applies_to = $3,
    name = $4,
    description = $5,
    values = $6,
    regex = $7,
    regex_hint = $8,
    updated_at = NOW()
WHERE
    tenant_id = $1 AND id = $2
RETURNING *;
