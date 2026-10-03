-- name: get-all-tags
select
    id,
    created_at,
    updated_at,
    name
from
    tags
where
    tenant_id = $1
order by
    name;

-- name: insert-tag
INSERT into
    tags (tenant_id, name)
values
    ($1, $2)
RETURNING *;

-- name: delete-tag
DELETE from
    tags
where
    tenant_id = $1 and id = $2;

-- name: update-tag
UPDATE
    tags
set
    name = $3,
    updated_at = now()
where
    tenant_id = $1 and id = $2
RETURNING *;
