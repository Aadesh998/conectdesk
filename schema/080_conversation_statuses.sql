-- Conversation statuses and priorities.

DROP TABLE IF EXISTS conversation_statuses CASCADE;
CREATE TABLE conversation_statuses (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	category conversation_status_category NOT NULL DEFAULT 'open',
	CONSTRAINT uniq_conversation_statuses_name UNIQUE (tenant_id, "name")
);
CREATE INDEX index_conversation_statuses_on_tenant_id ON conversation_statuses (tenant_id);

DROP TABLE IF EXISTS conversation_priorities CASCADE;
CREATE TABLE conversation_priorities (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	CONSTRAINT uniq_conversation_priorities_name UNIQUE (tenant_id, "name")
);
CREATE INDEX index_conversation_priorities_on_tenant_id ON conversation_priorities (tenant_id);
