-- Webhooks.

DROP TABLE IF EXISTS webhooks CASCADE;
CREATE TABLE webhooks (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	url TEXT NOT NULL,
	-- custom posts the raw event JSON; slack and teams post a chat message.
	kind TEXT NOT NULL DEFAULT 'custom',
	events webhook_event[] NOT NULL DEFAULT '{}',
	secret TEXT DEFAULT '',
	is_active BOOLEAN DEFAULT true,
	CONSTRAINT constraint_webhooks_on_name CHECK (length(name) <= 255),
	CONSTRAINT constraint_webhooks_on_url CHECK (length(url) <= 2048),
	CONSTRAINT constraint_webhooks_on_secret CHECK (length(secret) <= 255),
	CONSTRAINT constraint_webhooks_on_kind CHECK (kind IN ('custom', 'slack', 'teams')),
	CONSTRAINT constraint_webhooks_on_events_not_empty CHECK (array_length(events, 1) > 0)
);
CREATE INDEX index_webhooks_on_tenant_id ON webhooks (tenant_id);
