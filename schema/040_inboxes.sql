-- Inboxes: the email, live chat, WhatsApp, Messenger and Instagram channels conversations arrive on.

DROP TABLE IF EXISTS inboxes CASCADE;
CREATE TABLE inboxes (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"uuid" UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	"name" TEXT NOT NULL,
	deleted_at TIMESTAMPTZ NULL,
	channel channels NOT NULL,
	enabled bool DEFAULT TRUE NOT NULL,
	csat_enabled bool DEFAULT false NOT NULL,
	prompt_tags_on_reply bool DEFAULT false NOT NULL,
	config jsonb DEFAULT '{}'::jsonb NOT NULL,
	"from" TEXT NULL,
	from_name_template TEXT NOT NULL DEFAULT '',
	secret TEXT NULL,
	linked_email_inbox_id INT REFERENCES inboxes(id) ON DELETE SET NULL,
	CONSTRAINT constraint_inboxes_on_name CHECK (length("name") <= 140)
);
CREATE INDEX index_inboxes_on_tenant_id ON inboxes (tenant_id);
-- A WhatsApp number, Page or Instagram account can be connected to one
-- workspace: Meta's webhooks name only the account.
CREATE UNIQUE INDEX index_inboxes_on_channel_account ON inboxes (channel, (config->>'account_id'))
	WHERE deleted_at IS NULL AND config->>'account_id' IS NOT NULL;
