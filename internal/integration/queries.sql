-- name: get-integrations
SELECT * FROM chat_integrations WHERE tenant_id = $1 ORDER BY provider, created_at;

-- name: get-integration
SELECT * FROM chat_integrations WHERE tenant_id = $1 AND id = $2;

-- name: get-integration-by-channel
-- tenant: all, requests from Slack and Teams name the team and channel, not the
-- workspace; the row found says which workspace it is.
SELECT * FROM chat_integrations WHERE provider = $1 AND external_team_id = $2 AND channel_id = $3;

-- name: get-integrations-for-event
SELECT * FROM chat_integrations WHERE tenant_id = $1 AND is_active = true AND $2 = ANY(events);

-- name: upsert-integration
-- Connecting a channel again refreshes it, but only for the workspace it is
-- connected to; for another workspace nothing is returned.
INSERT INTO chat_integrations (tenant_id, provider, external_team_id, external_team_name, channel_id, channel_name,
	service_url, access_token, bot_user_id, events, connected_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (provider, external_team_id, channel_id) DO UPDATE SET
	external_team_name = EXCLUDED.external_team_name,
	channel_name = CASE WHEN EXCLUDED.channel_name <> '' THEN EXCLUDED.channel_name ELSE chat_integrations.channel_name END,
	service_url = EXCLUDED.service_url,
	access_token = EXCLUDED.access_token,
	bot_user_id = EXCLUDED.bot_user_id,
	connected_by = EXCLUDED.connected_by,
	is_active = true,
	updated_at = NOW()
WHERE chat_integrations.tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: update-integration
UPDATE chat_integrations SET events = $3, is_active = $4, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: update-integration-token
-- A Slack install into the same workspace issues a new bot token for all its channels.
UPDATE chat_integrations SET access_token = $3, bot_user_id = $4, updated_at = NOW()
WHERE provider = 'slack' AND external_team_id = $1 AND tenant_id = $2;

-- name: deactivate-team
-- tenant: all, Slack says the app was removed from a Slack workspace, which is
-- not tied to one of ours, so every channel of it is turned off.
UPDATE chat_integrations SET is_active = false, updated_at = NOW() WHERE provider = $1 AND external_team_id = $2;

-- name: delete-integration
DELETE FROM chat_integrations WHERE tenant_id = $1 AND id = $2;

-- name: get-thread
SELECT thread_ref FROM chat_integration_threads WHERE tenant_id = $1 AND integration_id = $2 AND conversation_id = $3;

-- name: insert-thread
INSERT INTO chat_integration_threads (tenant_id, integration_id, conversation_id, thread_ref)
VALUES ($1, $2, $3, $4)
ON CONFLICT (integration_id, conversation_id) DO NOTHING;

-- name: get-thread-conversation
SELECT conversation_id FROM chat_integration_threads WHERE tenant_id = $1 AND integration_id = $2 AND thread_ref = $3;
