package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_14_0 are the tenant tables added by V2_14_0.
var TenantScopedTablesV2_14_0 = []string{"custom_domains"}

// V2_14_0 adds custom_domains, which holds the hostnames a tenant has to prove
// it controls before they route to it. Domains already in use were set before
// verification existed and are kept working by recording them as verified.
func V2_14_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS custom_domains (
			id SERIAL PRIMARY KEY,
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			hostname TEXT NOT NULL,
			token TEXT NOT NULL,
			verified_at TIMESTAMPTZ NULL,
			checked_at TIMESTAMPTZ NULL,
			last_error TEXT NOT NULL DEFAULT '',
			CONSTRAINT constraint_custom_domains_on_hostname CHECK (hostname = LOWER(hostname) AND length(hostname) <= 253),
			CONSTRAINT uniq_custom_domains_tenant_id_id UNIQUE (tenant_id, id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_custom_domains_on_tenant_id_hostname ON custom_domains (tenant_id, hostname);
		CREATE UNIQUE INDEX IF NOT EXISTS index_custom_domains_on_verified_hostname ON custom_domains (hostname) WHERE verified_at IS NOT NULL;

		-- Keep existing domains working. The first tenant to hold a hostname keeps it.
		INSERT INTO custom_domains (tenant_id, hostname, token, verified_at, checked_at)
		SELECT DISTINCT ON (hostname) tenant_id, hostname, md5(random()::text), NOW(), NOW()
		FROM (
			SELECT id AS tenant_id, LOWER(custom_domain) AS hostname, 0 AS ord, id AS row_id
			FROM tenants WHERE custom_domain IS NOT NULL AND custom_domain <> ''
			UNION ALL
			SELECT tenant_id, LOWER(SUBSTRING(custom_domain FROM '^(?:[a-zA-Z][a-zA-Z0-9+.-]*://)?([^/:?#]+)')), 1, id
			FROM help_centers WHERE custom_domain <> ''
		) d
		WHERE hostname IS NOT NULL
		ORDER BY hostname, ord, row_id
		ON CONFLICT DO NOTHING;
	`)
	return err
}
