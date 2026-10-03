package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_16_0 are the tenant tables added by V2_16_0.
var TenantScopedTablesV2_16_0 = []string{"ai_usage"}

// V2_16_0 adds ai_usage, which counts each tenant's AI requests per month.
func V2_16_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_usage (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			"month" DATE NOT NULL,
			requests INT NOT NULL DEFAULT 0,
			PRIMARY KEY (tenant_id, "month")
		);
	`)
	return err
}
