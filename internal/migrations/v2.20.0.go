package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_20_0 are the tenant tables added by V2_20_0.
var TenantScopedTablesV2_20_0 = []string{"data_exports"}

// V2_20_0 adds data_exports, the tenant data exports admins download.
func V2_20_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`ALTER TYPE activity_log_type ADD VALUE IF NOT EXISTS 'data_exported'`); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS data_exports (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id SERIAL PRIMARY KEY,
			"uuid" uuid DEFAULT gen_random_uuid() NOT NULL UNIQUE,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			finished_at TIMESTAMPTZ NULL,
			expires_at TIMESTAMPTZ NULL,
			status TEXT NOT NULL DEFAULT 'running',
			requested_by INT NULL,
			requested_by_email TEXT NOT NULL DEFAULT '',
			media_uuid uuid NULL,
			"size" BIGINT NOT NULL DEFAULT 0,
			manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
			error TEXT NOT NULL DEFAULT '',
			CONSTRAINT constraint_data_exports_on_status CHECK (status IN ('running', 'ready', 'failed', 'expired')),
			CONSTRAINT uniq_data_exports_tenant_id_id UNIQUE (tenant_id, id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_data_exports_running ON data_exports (tenant_id) WHERE status = 'running';
		-- An export left running by a restart can't finish.
		UPDATE data_exports SET status = 'failed', error = 'interrupted' WHERE status = 'running';
	`)
	return err
}
