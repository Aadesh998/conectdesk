-- name: get-view
SELECT id, created_at, updated_at, name, filters, visibility, user_id, team_id
FROM views WHERE tenant_id = $1 AND id = $2;

-- name: get-user-views
-- Returns personal views (visibility='user') for a specific user
SELECT id, created_at, updated_at, name, filters, visibility, user_id, team_id
FROM views WHERE tenant_id = $1 AND user_id = $2 AND visibility = 'user'
ORDER BY name ASC;

-- name: get-shared-views-for-user
-- Returns shared views visible to a user (global + team views for user's teams)
SELECT id, created_at, updated_at, name, filters, visibility, user_id, team_id
FROM views
WHERE tenant_id = $1
  AND (visibility = 'all' OR (visibility = 'team' AND team_id = ANY($2)))
ORDER BY name ASC;

-- name: get-all-shared-views
SELECT id, created_at, updated_at, name, filters, visibility, user_id, team_id
FROM views
WHERE tenant_id = $1 AND visibility != 'user'
ORDER BY updated_at DESC;

-- name: insert-view
INSERT INTO views (tenant_id, name, filters, visibility, user_id, team_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: delete-view
DELETE FROM views
WHERE tenant_id = $1 AND id = $2;

-- name: update-view
UPDATE views
SET name = $3, filters = $4, visibility = $5, user_id = $6, team_id = $7, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;
