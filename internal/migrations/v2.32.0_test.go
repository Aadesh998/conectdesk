package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/stuffbin"
)

// TestV2_32_0 upgrades a database without the admissions tables and columns,
// checks they come back, that phone_key is derived from the number, that
// existing workspaces get the new settings while keeping any they have, and
// that the migration can run twice.
func TestV2_32_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_admissions")
	for _, tbl := range TenantScopedTablesV2_32_0 {
		db.MustExec(`DROP TABLE ` + tbl)
	}
	db.MustExec(`ALTER TABLE users DROP COLUMN phone_key, DROP COLUMN lead_stage, DROP COLUMN lead_stage_changed_at`)
	db.MustExec(`DELETE FROM settings WHERE key LIKE 'leads.%' OR key LIKE 'daily_report.%' OR key LIKE 'ai_features.%'`)
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme')`)
	db.MustExec(`INSERT INTO settings (tenant_id, key, value) VALUES (2, 'leads.auto_new', 'false')`)
	db.MustExec(`INSERT INTO users (tenant_id, type, first_name, phone_number) VALUES (1, 'contact', 'Asha', '+91 98765-43210'), (1, 'contact', 'Ravi', NULL)`)

	fs, err := stuffbin.NewLocalFS("/", "../../schema/890_seed_tenant.sql:"+SeedTenantFile)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := V2_32_0(db, fs, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var keys []string
	db.Select(&keys, `SELECT COALESCE(phone_key, '') FROM users WHERE first_name IN ('Asha', 'Ravi') ORDER BY first_name`)
	if len(keys) != 2 || keys[0] != "9876543210" || keys[1] != "" {
		t.Fatalf("phone keys = %v", keys)
	}

	var n int
	db.Get(&n, `SELECT COUNT(*) FROM settings WHERE key = ANY('{leads.stages,leads.auto_new,daily_report.enabled,ai_features.call_summaries}')`)
	if n != 8 {
		t.Fatalf("%d settings, want 4 for each of 2 workspaces", n)
	}
	var autoNew string
	db.Get(&autoNew, `SELECT value::TEXT FROM settings WHERE tenant_id = 2 AND key = 'leads.auto_new'`)
	if autoNew != "false" {
		t.Fatalf("existing leads.auto_new became %s", autoNew)
	}

	// The new tables take rows, and a second pending call-back for a contact is refused.
	db.MustExec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{}')`)
	db.MustExec(`INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		SELECT 1, u.id, i.id, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), 'T1'
		FROM users u, inboxes i WHERE u.first_name = 'Asha' AND i.name = 'Phone'`)
	insert := `INSERT INTO call_backs (tenant_id, contact_id, conversation_id, inbox_id, phone, last_call_sid, due_at)
		SELECT 1, c.contact_id, c.id, c.inbox_id, '+919876543210', 'sid', NOW() FROM conversations c WHERE c.reference_number = 'T1'`
	db.MustExec(insert)
	if _, err := db.Exec(insert); err == nil {
		t.Fatal("a second pending call-back was allowed")
	}
	db.MustExec(`INSERT INTO lead_stage_events (tenant_id, contact_id, to_stage) SELECT 1, id, 'new' FROM users WHERE first_name = 'Asha'`)
	db.MustExec(`INSERT INTO daily_report_runs (tenant_id, report_date) VALUES (1, CURRENT_DATE)`)
	db.MustExec(`INSERT INTO user_notifications (tenant_id, user_id, notification_type, title)
		SELECT 1, id, 'call_back_overdue', 'x' FROM users WHERE first_name = 'Asha'`)
}
