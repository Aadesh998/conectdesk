-- Browser calling: an agent's SIP account at Exotel, which their browser
-- signs in to so calls ring there instead of on their phone.

DROP TABLE IF EXISTS agent_sip_accounts CASCADE;
CREATE TABLE agent_sip_accounts (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	-- The SIP user and its password, encrypted, from Exotel.
	username TEXT NOT NULL,
	"password" TEXT NOT NULL,
	-- The SIP domain the browser registers to, and the host and port of the
	-- secure WebSocket it connects over; no host means the SIP domain.
	domain TEXT NOT NULL,
	server TEXT NOT NULL DEFAULT '',
	port INT NOT NULL DEFAULT 443,
	-- Whether calls ring the agent's browser; off, they ring their phone.
	ring_browser BOOLEAN NOT NULL DEFAULT TRUE,
	CONSTRAINT constraint_agent_sip_accounts_on_username CHECK (LENGTH(username) BETWEEN 1 AND 100),
	CONSTRAINT constraint_agent_sip_accounts_on_domain CHECK (LENGTH(domain) BETWEEN 1 AND 255),
	CONSTRAINT constraint_agent_sip_accounts_on_server CHECK (LENGTH(server) <= 255),
	CONSTRAINT constraint_agent_sip_accounts_on_port CHECK (port BETWEEN 1 AND 65535)
);
CREATE UNIQUE INDEX index_unique_agent_sip_accounts_on_user_id ON agent_sip_accounts (tenant_id, user_id);
CREATE UNIQUE INDEX index_unique_agent_sip_accounts_on_username ON agent_sip_accounts (tenant_id, LOWER(username));
CREATE INDEX index_agent_sip_accounts_on_tenant_id ON agent_sip_accounts (tenant_id);
