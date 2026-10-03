-- name: get-all-oidc
SELECT id, created_at, updated_at, name, provider_url, client_id, client_secret, enabled, provider, logo_url FROM oidc WHERE tenant_id = $1 ORDER BY updated_at DESC;

-- name: get-oidc
SELECT id, created_at, updated_at, name, provider_url, client_id, client_secret, enabled, provider, logo_url FROM oidc WHERE tenant_id = $1 AND id = $2;

-- name: insert-oidc
INSERT INTO oidc (tenant_id, name, provider, provider_url, client_id, client_secret, logo_url)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: update-oidc
UPDATE oidc
SET name = $3, provider = $4, provider_url = $5, client_id = $6, client_secret = $7, enabled = $8, logo_url = $9, updated_at = now()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-oidc
DELETE FROM oidc WHERE tenant_id = $1 AND id = $2;

-- name: get-all-enabled-oidc
-- tenant: all, the auth layer loads the enabled providers of every tenant; each row carries its tenant_id.
SELECT id, tenant_id, created_at, updated_at, name, provider_url, client_id, client_secret, enabled, provider, logo_url FROM oidc WHERE enabled = true ORDER BY id;
