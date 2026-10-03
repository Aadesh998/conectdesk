-- name: insert-export
-- A second export while one runs hits the unique index and returns no row.
INSERT INTO data_exports (tenant_id, requested_by, requested_by_email) VALUES ($1, $2, $3)
ON CONFLICT (tenant_id) WHERE status = 'running' DO NOTHING
RETURNING id, "uuid", created_at, finished_at, expires_at, status, requested_by_email, media_uuid, "size", manifest, error;

-- name: get-exports
SELECT id, "uuid", created_at, finished_at, expires_at, status, requested_by_email, media_uuid, "size", manifest, error
FROM data_exports WHERE tenant_id = $1 ORDER BY id DESC LIMIT 10;

-- name: get-export
SELECT id, "uuid", created_at, finished_at, expires_at, status, requested_by_email, media_uuid, "size", manifest, error
FROM data_exports WHERE tenant_id = $1 AND "uuid" = $2;

-- name: set-export-ready
UPDATE data_exports
SET status = 'ready', finished_at = NOW(), expires_at = NOW() + $5 * INTERVAL '1 hour', media_uuid = $3, "size" = $4, manifest = $6
WHERE tenant_id = $1 AND id = $2;

-- name: set-export-failed
UPDATE data_exports SET status = 'failed', finished_at = NOW(), error = $3 WHERE tenant_id = $1 AND id = $2;

-- name: get-expired-exports
-- tenant: all, the cleaner removes expired exports of every tenant, each under its own tenant_id.
SELECT tenant_id, id, media_uuid FROM data_exports WHERE status = 'ready' AND expires_at < NOW();

-- name: set-export-expired
UPDATE data_exports SET status = 'expired', media_uuid = NULL WHERE tenant_id = $1 AND id = $2;
