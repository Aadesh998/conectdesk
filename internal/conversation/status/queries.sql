-- name: get-status
select id,
    created_at,
    name,
    category
from conversation_statuses
where tenant_id = $1 and id = $2;

-- name: get-all-statuses
select id,
    created_at,
    name,
    category
from conversation_statuses
where tenant_id = $1;

-- name: insert-status
INSERT into conversation_statuses(tenant_id, name, category) values ($1, $2, $3) RETURNING *;

-- name: delete-status
DELETE from conversation_statuses where tenant_id = $1 and id = $2;

-- name: update-status
UPDATE conversation_statuses set name = $3, category = $4 where tenant_id = $1 and id = $2 RETURNING *;
