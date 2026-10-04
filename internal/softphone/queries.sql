-- name: get-account
SELECT user_id, username, "password", domain, server, port, ring_browser
FROM agent_sip_accounts
WHERE tenant_id = $1 AND user_id = $2;

-- name: upsert-account
-- An empty password keeps the one saved.
INSERT INTO agent_sip_accounts (tenant_id, user_id, username, "password", domain, server, port)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
	username = EXCLUDED.username,
	"password" = COALESCE(NULLIF(EXCLUDED."password", ''), agent_sip_accounts."password"),
	domain = EXCLUDED.domain,
	server = EXCLUDED.server,
	port = EXCLUDED.port,
	updated_at = NOW();

-- name: delete-account
DELETE FROM agent_sip_accounts WHERE tenant_id = $1 AND user_id = $2;

-- name: set-ring-browser
UPDATE agent_sip_accounts SET ring_browser = $3, updated_at = NOW()
WHERE tenant_id = $1 AND user_id = $2;
