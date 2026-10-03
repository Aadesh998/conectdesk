-- name: get-all
SELECT id, created_at, updated_at, name, description, permissions FROM roles WHERE tenant_id = $1;

-- name: get-role
SELECT id, created_at, updated_at, name, description, permissions FROM roles WHERE tenant_id = $1 AND id = $2;

-- name: delete-role
DELETE FROM roles WHERE tenant_id = $1 AND id = $2;

-- name: insert-role
INSERT INTO roles (tenant_id, name, description, permissions) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: update-role
UPDATE roles SET name = $3, description = $4, permissions = $5 WHERE tenant_id = $1 AND id = $2 RETURNING *;
