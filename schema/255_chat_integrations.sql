-- Slack channels and Microsoft Teams channels connected through the Slack app
-- and the Teams bot. Each posts the conversation events picked for it, and
-- agents act on those conversations from the channel.
DROP TABLE IF EXISTS chat_integrations CASCADE;
CREATE TABLE chat_integrations (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	provider TEXT NOT NULL,
	-- The Slack workspace's team ID, or the Microsoft 365 tenant ID.
	external_team_id TEXT NOT NULL,
	external_team_name TEXT NOT NULL DEFAULT '',
	channel_id TEXT NOT NULL,
	channel_name TEXT NOT NULL DEFAULT '',
	-- Teams: the Bot Framework address for the channel's region.
	service_url TEXT NOT NULL DEFAULT '',
	-- Slack: the bot token, encrypted, and the bot's own user ID.
	access_token TEXT NOT NULL DEFAULT '',
	bot_user_id TEXT NOT NULL DEFAULT '',
	events TEXT[] NOT NULL DEFAULT '{}',
	is_active BOOLEAN NOT NULL DEFAULT true,
	connected_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	CONSTRAINT constraint_chat_integrations_on_provider CHECK (provider IN ('slack', 'teams')),
	CONSTRAINT constraint_chat_integrations_on_channel_name CHECK (length(channel_name) <= 255),
	CONSTRAINT constraint_chat_integrations_on_team_name CHECK (length(external_team_name) <= 255)
);
-- A channel belongs to one workspace, so its events and actions go to one place.
CREATE UNIQUE INDEX index_chat_integrations_on_channel ON chat_integrations (provider, external_team_id, channel_id);
CREATE INDEX index_chat_integrations_on_tenant_id ON chat_integrations (tenant_id);

-- The thread a conversation's updates go to in a connected channel.
DROP TABLE IF EXISTS chat_integration_threads CASCADE;
CREATE TABLE chat_integration_threads (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	integration_id INT NOT NULL REFERENCES chat_integrations(id) ON DELETE CASCADE ON UPDATE CASCADE,
	conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE,
	-- Slack: the first message's ts. Teams: the thread's conversation ID.
	thread_ref TEXT NOT NULL
);
CREATE UNIQUE INDEX index_chat_integration_threads_on_conversation ON chat_integration_threads (integration_id, conversation_id);
CREATE UNIQUE INDEX index_chat_integration_threads_on_ref ON chat_integration_threads (integration_id, thread_ref);
CREATE INDEX index_chat_integration_threads_on_tenant_id ON chat_integration_threads (tenant_id);
