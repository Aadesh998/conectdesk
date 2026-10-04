package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_35_0 are the tenant tables added by V2_35_0.
var TenantScopedTablesV2_35_0 = []string{"agent_sip_accounts"}

// V2_35_0 adds agent_sip_accounts, the Exotel SIP account an agent's browser
// signs in to for browser calling.
func V2_35_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_sip_accounts (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			user_id BIGINT NOT NULL,
			username TEXT NOT NULL,
			"password" TEXT NOT NULL,
			domain TEXT NOT NULL,
			server TEXT NOT NULL DEFAULT '',
			port INT NOT NULL DEFAULT 443,
			ring_browser BOOLEAN NOT NULL DEFAULT TRUE,
			CONSTRAINT constraint_agent_sip_accounts_on_username CHECK (LENGTH(username) BETWEEN 1 AND 100),
			CONSTRAINT constraint_agent_sip_accounts_on_domain CHECK (LENGTH(domain) BETWEEN 1 AND 255),
			CONSTRAINT constraint_agent_sip_accounts_on_server CHECK (LENGTH(server) <= 255),
			CONSTRAINT constraint_agent_sip_accounts_on_port CHECK (port BETWEEN 1 AND 65535),
			CONSTRAINT uniq_agent_sip_accounts_tenant_id_id UNIQUE (tenant_id, id),
			-- Tenant-scoped, like every reference between tenant tables.
			FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_unique_agent_sip_accounts_on_user_id ON agent_sip_accounts (tenant_id, user_id);
		CREATE UNIQUE INDEX IF NOT EXISTS index_unique_agent_sip_accounts_on_username ON agent_sip_accounts (tenant_id, LOWER(username));
		CREATE INDEX IF NOT EXISTS index_agent_sip_accounts_on_tenant_id ON agent_sip_accounts (tenant_id);
	`)
	return err
}
