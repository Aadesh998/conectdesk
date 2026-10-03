-- Delivery and read receipts for messages sent on WhatsApp, Messenger and
-- Instagram: each platform message ID a send returned, and how far it got.
DROP TABLE IF EXISTS message_deliveries CASCADE;
CREATE TABLE message_deliveries (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	message_id BIGINT NOT NULL REFERENCES conversation_messages(id) ON DELETE CASCADE ON UPDATE CASCADE,
	inbox_id INT NOT NULL REFERENCES inboxes(id) ON DELETE CASCADE ON UPDATE CASCADE,
	-- The contact's ID on the platform, which a Messenger read receipt names.
	recipient TEXT NOT NULL,
	external_id TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'sent',
	error TEXT NULL,
	CONSTRAINT constraint_message_deliveries_on_status CHECK (status IN ('sent', 'delivered', 'read', 'failed'))
);
CREATE UNIQUE INDEX index_message_deliveries_on_inbox_external_id ON message_deliveries (inbox_id, external_id);
CREATE INDEX index_message_deliveries_on_message_id ON message_deliveries (message_id);
CREATE INDEX index_message_deliveries_on_inbox_recipient ON message_deliveries (inbox_id, recipient, created_at);
CREATE INDEX index_message_deliveries_on_tenant_id ON message_deliveries (tenant_id);
