package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_33_0 are the tenant tables added by V2_33_0.
var TenantScopedTablesV2_33_0 = []string{"report_copilot_messages"}

// V2_33_0 adds report_copilot_messages, an agent's copilot chat on a report
// such as the SLA dashboard.
func V2_33_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS report_copilot_messages (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			user_id BIGINT NOT NULL,
			scope TEXT NOT NULL,
			"role" TEXT NOT NULL,
			content TEXT NOT NULL,
			CONSTRAINT constraint_report_copilot_messages_on_scope CHECK (scope IN ('sla')),
			CONSTRAINT constraint_report_copilot_messages_on_role CHECK ("role" IN ('user', 'assistant')),
			CONSTRAINT uniq_report_copilot_messages_tenant_id_id UNIQUE (tenant_id, id),
			-- Tenant-scoped, like every reference between tenant tables.
			FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE
		);
		CREATE INDEX IF NOT EXISTS index_report_copilot_messages_on_user_scope ON report_copilot_messages (tenant_id, user_id, scope, id);
		CREATE INDEX IF NOT EXISTS index_report_copilot_messages_on_tenant_id ON report_copilot_messages (tenant_id);
	`)
	return err
}
