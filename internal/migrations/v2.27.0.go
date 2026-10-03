package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_27_0 are the tenant tables added by V2_27_0.
var TenantScopedTablesV2_27_0 = []string{"message_deliveries"}

// V2_27_0 adds message_deliveries, which ties the IDs WhatsApp, Messenger and
// Instagram give a sent message to the message, so their delivery and read
// receipts can be shown on it.
func V2_27_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS message_deliveries (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			message_id BIGINT NOT NULL,
			inbox_id INT NOT NULL,
			recipient TEXT NOT NULL,
			external_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'sent',
			error TEXT NULL,
			CONSTRAINT constraint_message_deliveries_on_status CHECK (status IN ('sent', 'delivered', 'read', 'failed')),
			CONSTRAINT uniq_message_deliveries_tenant_id_id UNIQUE (tenant_id, id),
			-- Tenant-scoped, like every reference between tenant tables.
			FOREIGN KEY (tenant_id, message_id) REFERENCES conversation_messages (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, inbox_id) REFERENCES inboxes (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_message_deliveries_on_inbox_external_id ON message_deliveries (inbox_id, external_id);
		CREATE INDEX IF NOT EXISTS index_message_deliveries_on_message_id ON message_deliveries (message_id);
		CREATE INDEX IF NOT EXISTS index_message_deliveries_on_inbox_recipient ON message_deliveries (inbox_id, recipient, created_at);
		CREATE INDEX IF NOT EXISTS index_message_deliveries_on_tenant_id ON message_deliveries (tenant_id);
	`)
	return err
}
